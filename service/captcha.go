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
	"math"
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

	captchaKeyPrefix   = "captcha"
	captchaCodeLength  = 5
	captchaAlphabet    = "abcdefghjkmnpqrstuvwxyz23456789"
	captchaImageWidth  = 220
	captchaImageHeight = 56
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
		glyph := renderCaptchaGlyph(face, ch, rng)
		centerX := float64(24 + i*42 + rng.Intn(4))
		centerY := float64(27 + rng.Intn(3))
		angle := (rng.Float64()*2 - 1) * 0.20
		scale := 0.92 + rng.Float64()*0.16
		shearX := (rng.Float64()*2 - 1) * 0.18
		shearY := (rng.Float64()*2 - 1) * 0.10
		drawTransformedCaptchaGlyph(img, glyph, centerX, centerY, angle, scale, shearX, shearY)
	}

	// Bend the whole image with a low-frequency wave so both the glyphs and
	// their baselines are distorted. This is deliberately mild enough to keep
	// the answer readable for humans but defeats straight-line template OCR.
	img = warpCaptchaImage(img, rng)

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

const (
	captchaGlyphCellSize        = 64
	captchaGlyphVerticalStretch = 1.12
)

func renderCaptchaGlyph(face font.Face, ch rune, rng *mathrand.Rand) *image.RGBA {
	const padding = 3
	bounds, _ := font.BoundString(face, string(ch))
	minX, minY := bounds.Min.X.Floor(), bounds.Min.Y.Floor()
	maxX, maxY := bounds.Max.X.Ceil(), bounds.Max.Y.Ceil()
	width := maxX - minX + padding*2
	height := maxY - minY + padding*2
	if width <= 0 || height <= 0 {
		width, height = captchaGlyphCellSize, captchaGlyphCellSize
		minX, minY = 0, 0
	}
	glyph := image.NewRGBA(image.Rect(0, 0, width, height))
	drawer := &font.Drawer{
		Dst:  glyph,
		Src:  image.NewUniform(darkCaptchaColor(rng)),
		Face: face,
		Dot:  fixed.P(padding-minX, padding-minY),
	}
	drawer.DrawString(string(ch))
	return glyph
}

func drawTransformedCaptchaGlyph(dst *image.RGBA, glyph *image.RGBA, centerX, centerY, angle, scale, shearX, shearY float64) {
	cosine := math.Cos(angle)
	sine := math.Sin(angle)
	halfWidth := float64(glyph.Bounds().Dx()) / 2
	halfHeight := float64(glyph.Bounds().Dy()) / 2
	radius := math.Hypot(halfWidth, halfHeight*captchaGlyphVerticalStretch)*1.2 + 4
	minX := captchaMaxInt(0, int(centerX-radius))
	maxX := captchaMinInt(captchaImageWidth-1, int(centerX+radius))
	minY := captchaMaxInt(0, int(centerY-radius))
	maxY := captchaMinInt(captchaImageHeight-1, int(centerY+radius))
	glyphCenterX := float64(glyph.Bounds().Dx()) / 2
	glyphCenterY := float64(glyph.Bounds().Dy()) / 2

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			deltaX := (float64(x) + 0.5 - centerX) / scale
			deltaY := (float64(y) + 0.5 - centerY) / (scale * captchaGlyphVerticalStretch)

			// Inverse shear followed by inverse rotation. The forward transform
			// shears x by y and y by x, then rotates around the glyph center.
			unshearedX := deltaX - shearX*deltaY
			unshearedY := deltaY - shearY*deltaX
			rotatedX := cosine*unshearedX + sine*unshearedY
			rotatedY := -sine*unshearedX + cosine*unshearedY

			r, g, b, a := sampleCaptchaPixel(glyph, glyphCenterX+rotatedX, glyphCenterY+rotatedY)
			if a == 0 {
				continue
			}
			blendCaptchaPixel(dst, x, y, r, g, b, a)
		}
	}
}

func warpCaptchaImage(src *image.RGBA, rng *mathrand.Rand) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	phaseX := rng.Float64() * 2 * math.Pi
	phaseY := rng.Float64() * 2 * math.Pi
	amplitudeX := 2.2 + rng.Float64()*1.8
	amplitudeY := 1.6 + rng.Float64()*1.4
	periodX := 52 + rng.Float64()*26
	periodY := 38 + rng.Float64()*18

	for y := 0; y < captchaImageHeight; y++ {
		for x := 0; x < captchaImageWidth; x++ {
			sourceX := float64(x) +
				amplitudeX*math.Sin(2*math.Pi*float64(y)/periodY+phaseX) +
				1.1*math.Sin(2*math.Pi*float64(y)/19+phaseY)
			sourceY := float64(y) +
				amplitudeY*math.Sin(2*math.Pi*float64(x)/periodX+phaseY)
			r, g, b, _ := sampleCaptchaPixel(src, sourceX, sourceY)
			dst.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return dst
}

func sampleCaptchaPixel(img *image.RGBA, x, y float64) (uint8, uint8, uint8, uint8) {
	bounds := img.Bounds()
	if x < float64(bounds.Min.X) {
		x = float64(bounds.Min.X)
	}
	if y < float64(bounds.Min.Y) {
		y = float64(bounds.Min.Y)
	}
	if x > float64(bounds.Max.X-1) {
		x = float64(bounds.Max.X - 1)
	}
	if y > float64(bounds.Max.Y-1) {
		y = float64(bounds.Max.Y - 1)
	}

	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := captchaMinInt(x0+1, bounds.Max.X-1)
	y1 := captchaMinInt(y0+1, bounds.Max.Y-1)
	fx := x - float64(x0)
	fy := y - float64(y0)

	var sumAlpha, sumRed, sumGreen, sumBlue float64
	weights := [4]float64{
		(1 - fx) * (1 - fy), fx * (1 - fy), (1 - fx) * fy, fx * fy,
	}
	points := [4]image.Point{
		{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x0, Y: y1}, {X: x1, Y: y1},
	}
	for i, point := range points {
		pixel := img.RGBAAt(point.X, point.Y)
		alpha := float64(pixel.A) * weights[i]
		sumAlpha += alpha
		sumRed += float64(pixel.R) * alpha
		sumGreen += float64(pixel.G) * alpha
		sumBlue += float64(pixel.B) * alpha
	}
	if sumAlpha == 0 {
		return 0, 0, 0, 0
	}
	return uint8(sumRed / sumAlpha), uint8(sumGreen / sumAlpha), uint8(sumBlue / sumAlpha), uint8(sumAlpha)
}

func blendCaptchaPixel(dst *image.RGBA, x, y int, r, g, b, a uint8) {
	alpha := float64(a) / 255
	background := dst.RGBAAt(x, y)
	dst.SetRGBA(x, y, color.RGBA{
		R: uint8(float64(r)*alpha + float64(background.R)*(1-alpha)),
		G: uint8(float64(g)*alpha + float64(background.G)*(1-alpha)),
		B: uint8(float64(b)*alpha + float64(background.B)*(1-alpha)),
		A: 255,
	})
}

func captchaMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func captchaMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
			Size:    44,
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
