package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/big"
	mathrand "math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	// CaptchaTTL is deliberately short: a challenge is a single action, not a
	// session credential. Five minutes is enough for a human and keeps Redis
	// growth bounded.
	CaptchaTTL = 5 * time.Minute

	captchaKeyPrefix    = "captcha"
	captchaCodeLength   = 5
	captchaAlphabet     = "abcdefghjkmnpqrstuvwxyz23456789"
	captchaImageWidth   = 220
	captchaImageHeight  = 80
	captchaImagePadding = 16
)

var (
	captchaTypefaceOnce sync.Once
	captchaTypeface     *opentype.Font
)

// GenerateCaptcha returns an opaque challenge id and a PNG data URL. The answer
// is kept server-side and is removed on the first verification attempt, so a
// bot gets exactly one guess per image.
func GenerateCaptcha() (challengeID string, imageDataURL string, err error) {
	answer, err := randomCaptchaCode()
	if err != nil {
		return "", "", err
	}
	id, err := randomCaptchaID()
	if err != nil {
		return "", "", err
	}
	pngBytes, err := renderCaptchaImage(answer)
	if err != nil {
		return "", "", err
	}
	if err := smsCodeStoreForUse().Set(context.Background(), captchaKey(id), strings.ToLower(answer), CaptchaTTL); err != nil {
		return "", "", fmt.Errorf("store CAPTCHA challenge: %w", err)
	}
	return id, "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes), nil
}

// VerifyCaptcha accepts the "id:answer" token produced by the frontend. A
// missing, expired, malformed, or already-used token is rejected. The stored
// answer is deleted before comparison so a concurrent second request cannot
// reuse it.
func VerifyCaptcha(ctx context.Context, token string) bool {
	id, answer, ok := strings.Cut(strings.TrimSpace(token), ":")
	if !ok || id == "" || answer == "" || len(id) > 128 {
		return false
	}
	stored, ok, err := smsCodeStoreForUse().GetDel(ctx, captchaKey(id))
	if err != nil || !ok {
		return false
	}
	return subtle.ConstantTimeCompare(
		[]byte(strings.ToLower(strings.TrimSpace(answer))),
		[]byte(stored),
	) == 1
}

func captchaKey(id string) string {
	return captchaKeyPrefix + ":" + id
}

func randomCaptchaCode() (string, error) {
	var builder strings.Builder
	builder.Grow(captchaCodeLength)
	limit := big.NewInt(int64(len(captchaAlphabet)))
	for i := 0; i < captchaCodeLength; i++ {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate CAPTCHA code: %w", err)
		}
		builder.WriteByte(captchaAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func randomCaptchaID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate CAPTCHA id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func renderCaptchaImage(code string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, captchaImageWidth, captchaImageHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{R: 247, G: 247, B: 242, A: 255}}, image.Point{}, draw.Src)

	rng := mathrand.New(mathrand.NewSource(time.Now().UnixNano() ^ int64(os.Getpid())))
	for i := 0; i < 380; i++ {
		img.Set(
			rng.Intn(captchaImageWidth),
			rng.Intn(captchaImageHeight),
			color.RGBA{
				R: uint8(120 + rng.Intn(100)),
				G: uint8(120 + rng.Intn(100)),
				B: uint8(120 + rng.Intn(100)),
				A: 255,
			},
		)
	}
	for i := 0; i < 7; i++ {
		drawCaptchaLine(img, rng, color.RGBA{
			R: uint8(140 + rng.Intn(80)),
			G: uint8(140 + rng.Intn(80)),
			B: uint8(140 + rng.Intn(80)),
			A: 255,
		})
	}

	face := newCaptchaFace()
	for i, ch := range code {
		dot := fixed.P(
			captchaImagePadding+i*38+rng.Intn(5),
			52+rng.Intn(11)-5,
		)
		drawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(darkCaptchaColor(rng)),
			Face: face,
			Dot:  dot,
		}
		drawer.DrawString(string(ch))
	}

	// A few lines over the glyphs make naive OCR and simple template matching
	// less useful than the plain rendered text.
	for i := 0; i < 4; i++ {
		drawCaptchaLine(img, rng, color.RGBA{
			R: uint8(80 + rng.Intn(80)),
			G: uint8(80 + rng.Intn(80)),
			B: uint8(80 + rng.Intn(80)),
			A: 255,
		})
	}

	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode CAPTCHA image: %w", err)
	}
	return buf.Bytes(), nil
}

func newCaptchaFace() font.Face {
	captchaTypefaceOnce.Do(func() {
		parsed, err := opentype.Parse(goregular.TTF)
		if err == nil {
			captchaTypeface = parsed
		}
	})
	if captchaTypeface != nil {
		face, err := opentype.NewFace(captchaTypeface, &opentype.FaceOptions{
			Size:    34,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err == nil {
			return face
		}
	}
	return basicfont.Face7x13
}

func darkCaptchaColor(rng *mathrand.Rand) color.RGBA {
	return color.RGBA{
		R: uint8(20 + rng.Intn(100)),
		G: uint8(20 + rng.Intn(100)),
		B: uint8(20 + rng.Intn(100)),
		A: 255,
	}
}

func drawCaptchaLine(img *image.RGBA, rng *mathrand.Rand, c color.RGBA) {
	x0 := rng.Intn(captchaImageWidth)
	y0 := rng.Intn(captchaImageHeight)
	x1 := rng.Intn(captchaImageWidth)
	y1 := rng.Intn(captchaImageHeight)

	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy
	for {
		img.Set(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
