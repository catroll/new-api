package model

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestValidateChannelGroupList(t *testing.T) {
	// Four 63-rune names plus three commas fill the list budget exactly.
	fullList := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 63),
	}, ",")
	require.Len(t, []rune(fullList), ChannelGroupListColumnWidth)

	sixtyFour := strings.Repeat("长", ChannelGroupNameColumnWidth)
	sixtyFive := strings.Repeat("长", ChannelGroupNameColumnWidth+1)

	tests := []struct {
		name    string
		group   string
		wantErr string
	}{
		{name: "empty", group: ""},
		{name: "single name", group: "default"},
		{name: "several names", group: "default,vip,svip"},
		{name: "spaces around names", group: "default, vip , svip"},
		{name: "list exactly at the width", group: fullList},
		{name: "list one rune over the width", group: fullList + "d", wantErr: "group list must not exceed 255 characters"},
		{name: "group name exactly at the width", group: sixtyFour},
		{name: "group name one rune over the width", group: sixtyFive, wantErr: "must not exceed 64 characters"},
		{name: "over-wide name inside a valid list", group: "default," + sixtyFive, wantErr: "must not exceed 64 characters"},
		{name: "server default", group: "default,vip,svip,glm-5.2限时优惠,glm-5.3限时优惠,deepseek-v4.1-flash限时优惠"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChannelGroupList(tt.group)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestChannelUpdateAcceptsLongGroupList covers the regression behind
// `1406 Data too long for column 'group'`: a channel that holds more group
// names than a varchar(64) column fits, which used to reject the whole save.
func TestChannelUpdateAcceptsLongGroupList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	groups := []string{
		"default",
		"vip",
		"svip",
		"glm-5.2限时优惠",
		"glm-5.3限时优惠",
		"deepseek-v4.1-flash限时优惠",
		"deepseek-v4-flash限时优惠",
	}
	groupList := strings.Join(groups, ",")
	require.Greater(t, len([]rune(groupList)), ChannelGroupNameColumnWidth,
		"the regression needs a list that does not fit a varchar(64) column")

	channel := &Channel{Name: "long-groups", Key: "sk-test", Models: "m1,m2", Group: "default", Status: 1}
	require.NoError(t, channel.Insert())

	channel.Group = groupList
	require.NoError(t, channel.Update())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, groupList, stored.Group)

	var abilities int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilities).Error)
	assert.EqualValues(t, len(groups)*2, abilities, "every group x model pair must have an ability row")

	var names []string
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).
		Distinct().Order("`group`").Pluck("`group`", &names).Error)
	assert.ElementsMatch(t, groups, names)
}

func TestChannelWriteRejectsOverwideGroupList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	channel := &Channel{Name: "too-wide", Key: "sk-test", Models: "m1", Status: 1}
	require.NoError(t, channel.Insert())

	channel.Group = strings.Repeat("a", ChannelGroupListColumnWidth+1)
	err = channel.Update()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group list must not exceed 255 characters")

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, "default", stored.Group, "a rejected save must not touch the row")
}

// TestWidenChannelGroupColumnSQLite documents that SQLite needs no widening:
// it ignores varchar lengths, so the migration must be a no-op rather than an
// unsupported ALTER.
func TestWidenChannelGroupColumnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))

	recorder := &migrationSQLRecorder{}
	require.NoError(t, widenChannelGroupColumn(db.Session(&gorm.Session{Logger: recorder}), "channels"))
	assert.Empty(t, recorder.schemaMutations())
}

func testWidenChannelGroupColumn(t *testing.T, db *gorm.DB) {
	t.Helper()

	groupCol := "`group`"
	if db.Dialector.Name() == "postgres" {
		groupCol = `"group"`
	}
	table := fmt.Sprintf("channel_group_widen_%d", time.Now().UnixNano())
	require.NoError(t, db.Exec(fmt.Sprintf("CREATE TABLE %s (%s varchar(64) DEFAULT 'default')", table, groupCol)).Error)
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)).Error })

	// A value at the old width round-trips before the migration.
	atLimit := strings.Repeat("长", 64)
	require.NoError(t, db.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (?)", table, groupCol), atLimit).Error)
	// The width is what made the admin save fail: 70 runes no longer fit.
	overLimit := strings.Repeat("长", 70)
	require.Error(t, db.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (?)", table, groupCol), overLimit).Error)

	require.NoError(t, widenChannelGroupColumn(db, table))

	columnTypes, err := db.Migrator().ColumnTypes(table)
	require.NoError(t, err)
	var width int64
	for _, columnType := range columnTypes {
		if columnType.Name() != "group" {
			continue
		}
		width, _ = columnType.Length()
	}
	assert.EqualValues(t, ChannelGroupListColumnWidth, width)

	require.NoError(t, db.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (?)", table, groupCol), overLimit).Error)

	var stored []string
	require.NoError(t, db.Raw(fmt.Sprintf("SELECT %s FROM %s", groupCol, table)).Scan(&stored).Error)
	assert.Equal(t, []string{atLimit, overLimit}, stored)

	// A second startup must not run the ALTER again.
	recorder := &migrationSQLRecorder{}
	require.NoError(t, widenChannelGroupColumn(db.Session(&gorm.Session{Logger: recorder}), table))
	assert.Empty(t, recorder.schemaMutations())
}

func TestWidenChannelGroupColumnMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	testWidenChannelGroupColumn(t, db)
}

func TestWidenChannelGroupColumnPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	testWidenChannelGroupColumn(t, db)
}
