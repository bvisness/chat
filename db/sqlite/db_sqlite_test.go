//go:build cgo

package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/bvisness/chat/db"
	"github.com/bvisness/chat/utils"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// [db.DefaultScan] is supposed to mirror the behavior of [sql.Rows.Scan], but
// we cannot reasonably test this without a real db driver. SQLite provides a
// nice in-memory way to do this, however.
func TestDefaultScan(t *testing.T) {
	conn := utils.Must1(sql.Open("sqlite3", ":memory:"))
	utils.Must1(conn.Exec("CREATE TABLE times (d DATETIME)"))
	utils.Must1(conn.Exec("INSERT INTO times VALUES (?)", testTime))

	assertEquivalentOnAGazillionThings[string](t, conn)
	assertEquivalentOnAGazillionThings[bool](t, conn)
	assertEquivalentOnAGazillionThings[int](t, conn)
	assertEquivalentOnAGazillionThings[int8](t, conn)
	assertEquivalentOnAGazillionThings[int16](t, conn)
	assertEquivalentOnAGazillionThings[int32](t, conn)
	assertEquivalentOnAGazillionThings[int64](t, conn)
	assertEquivalentOnAGazillionThings[uint](t, conn)
	assertEquivalentOnAGazillionThings[uint8](t, conn)
	assertEquivalentOnAGazillionThings[uint16](t, conn)
	assertEquivalentOnAGazillionThings[uint32](t, conn)
	assertEquivalentOnAGazillionThings[uint64](t, conn)
	assertEquivalentOnAGazillionThings[time.Time](t, conn)
}

func assertEquivalentOnAGazillionThings[T any](t *testing.T, conn *sql.DB) {
	t.Logf("===== %s =====", reflect.TypeFor[T]())

	// The Go values here are explicitly using the types mandated by
	// [sql.Scanner].
	assertEquivalent[T](t, conn, "SELECT 'hello'", "hello")
	assertEquivalent[T](t, conn, "SELECT ''", "")
	assertEquivalent[T](t, conn, "SELECT '42'", "42")
	assertEquivalent[T](t, conn, "SELECT '3.14'", "3.14")
	assertEquivalent[T](t, conn, "SELECT 'true'", "true")
	assertEquivalent[T](t, conn, "SELECT TRUE", 1) // this is a bit jank, because sqlite doesn't do booleans
	assertEquivalent[T](t, conn, "SELECT 0", int64(0))
	assertEquivalent[T](t, conn, "SELECT 8", int64(8))
	assertEquivalent[T](t, conn, "SELECT -1", int64(-1))
	assertEquivalent[T](t, conn, "SELECT 2147483648", int64(2147483648))
	assertEquivalent[T](t, conn, "SELECT 9223372036854775807", int64(9223372036854775807))
	assertEquivalent[T](t, conn, "SELECT 3.14", float64(3.14))
	assertEquivalent[T](t, conn, "SELECT 0.0", float64(0))
	assertEquivalent[T](t, conn, "SELECT 1e100", float64(1e100))
	assertEquivalent[T](t, conn, "SELECT x'deadbeef'", []byte{0xDE, 0xAD, 0xBE, 0xEF})
	assertEquivalent[T](t, conn, "SELECT d FROM times", testTime)
	assertEquivalent[T](t, conn, "SELECT '2024-01-02T03:04:05Z'", "2024-01-02T03:04:05Z")
	assertEquivalent[T](t, conn, "SELECT NULL", nil)
}

func assertEquivalent[T any](t *testing.T, conn *sql.DB, query string, goValue any) {
	t.Logf("%#v:", goValue)

	var dest1, dest2 T
	var err1, err2 error

	row := conn.QueryRow(query)
	err1 = row.Scan(&dest1)

	err2 = db.DefaultScan(goValue, &dest2)

	t.Logf("  Value 1: %#v", dest1)
	t.Logf("  Value 2: %#v", dest2)
	t.Logf("  Error 1: %v", err1)
	t.Logf("  Error 2: %v", err2)
	require.Equal(t, dest1, dest2)
	require.Equal(t, err1 == nil, err2 == nil)
}

func TestIntegration(t *testing.T) {
	type Identifiable struct {
		ID int `db:"id"`
	}
	type Asset struct {
		Hash string `db:"hash"`
	}
	type User struct {
		Identifiable
		Name          string  `db:"name"`
		Nickname      *string `db:"nickname"`
		ProfilePicID  int     `db:"profile_pic_id"`
		BannerImageID *int    `db:"banner_img_id"`
	}

	conn := utils.Must1(sql.Open("sqlite3", ":memory:"))
	utils.Must1(conn.Exec(`
		CREATE TABLE asset (
			id                INTEGER PRIMARY KEY,
			hash              TEXT NOT NULL
		);
		CREATE TABLE user (
			id                TEXT PRIMARY KEY,
			name              TEXT NOT NULL,
			nickname          TEXT,
			profile_pic_id    INTEGER NOT NULL REFERENCES asset(id), 
			banner_img_id     INTEGER  				 REFERENCES asset(id)
		);
		INSERT INTO asset(id, hash) VALUES
			(1, '600df00d'),
			(2, 'baadf00d'),
			(3, 'deadbeef');
		INSERT INTO user(id, name, nickname, profile_pic_id, banner_img_id) VALUES
			(1, 'Alice', NULL, 1, NULL),
			(2, 'Bob', 'Robert', 2, 3);
	`))

	t.Run("single table", func(t *testing.T) {
		users, err := db.Query[User](context.Background(), conn, `SELECT $columns FROM user`)
		require.Nil(t, err)
		assert.Equal(t, []*User{
			{
				Identifiable: Identifiable{1},
				Name:         "Alice",
				ProfilePicID: 1,
			},
			{
				Identifiable:  Identifiable{2},
				Name:          "Bob",
				Nickname:      new("Robert"),
				ProfilePicID:  2,
				BannerImageID: new(3),
			},
		}, users)
	})

	t.Run("joins", func(t *testing.T) {
		type userAndAssets struct {
			User       User   `db:"user"`
			ProfilePic Asset  `db:"profile_pic"`
			BannerImg  *Asset `db:"banner_img"`
		}
		usersWithAssets, err := db.Query[userAndAssets](context.Background(), conn, `
			SELECT $columns
			FROM
				user
				LEFT JOIN asset AS profile_pic ON user.profile_pic_id = profile_pic.id
				LEFT JOIN asset AS banner_img ON user.banner_img_id = banner_img.id
		`)
		require.Nil(t, err)
		assert.Equal(t, []*userAndAssets{
			{
				User: User{
					Identifiable: Identifiable{1},
					Name:         "Alice",
					ProfilePicID: 1,
				},
				ProfilePic: Asset{
					Hash: "600df00d",
				},
			},
			{
				User: User{
					Identifiable:  Identifiable{2},
					Name:          "Bob",
					Nickname:      new("Robert"),
					ProfilePicID:  2,
					BannerImageID: new(3),
				},
				ProfilePic: Asset{
					Hash: "baadf00d",
				},
				BannerImg: &Asset{
					Hash: "deadbeef",
				},
			},
		}, usersWithAssets)
	})
}
