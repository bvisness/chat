package db

import (
	"database/sql"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateScanArgs(t *testing.T) {
	t.Run("primitives", func(t *testing.T) {
		type s struct {
			A  string    `db:"a"`
			A2 string    // no tag, should be ignored
			B  int       `db:"b"`
			C  int8      `db:"c"`
			D  int16     `db:"d"`
			E  int32     `db:"e"`
			F  int64     `db:"f"`
			G  uint      `db:"g"`
			H  uint8     `db:"h"`
			I  uint16    `db:"i"`
			J  uint32    `db:"j"`
			K  uint64    `db:"k"`
			L  bool      `db:"l"`
			M  float32   `db:"m"`
			N  float64   `db:"n"`
			O  []byte    `db:"o"`
			P  time.Time `db:"p"`
			q  int       `db:"q"` // not exported, should be ignored
		}
		out := new(s)
		args := generateScanArgs(out)

		assert.Len(t, args, 16)

		now := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
		*args[0].(*string) = "hello"
		*args[1].(*int) = 1
		*args[2].(*int8) = 2
		*args[3].(*int16) = 3
		*args[4].(*int32) = 4
		*args[5].(*int64) = 5
		*args[6].(*uint) = 6
		*args[7].(*uint8) = 7
		*args[8].(*uint16) = 8
		*args[9].(*uint32) = 9
		*args[10].(*uint64) = 10
		*args[11].(*bool) = true
		*args[12].(*float32) = 1.5
		*args[13].(*float64) = 2.5
		*args[14].(*[]byte) = []byte("world")
		*args[15].(*time.Time) = now

		assert.Equal(t, "hello", out.A)
		assert.Equal(t, 1, out.B)
		assert.Equal(t, int8(2), out.C)
		assert.Equal(t, int16(3), out.D)
		assert.Equal(t, int32(4), out.E)
		assert.Equal(t, int64(5), out.F)
		assert.Equal(t, uint(6), out.G)
		assert.Equal(t, uint8(7), out.H)
		assert.Equal(t, uint16(8), out.I)
		assert.Equal(t, uint32(9), out.J)
		assert.Equal(t, uint64(10), out.K)
		assert.Equal(t, true, out.L)
		assert.Equal(t, float32(1.5), out.M)
		assert.Equal(t, 2.5, out.N)
		assert.Equal(t, []byte("world"), out.O)
		assert.Equal(t, now, out.P)
	})
	t.Run("pointers to primitives", func(t *testing.T) {
		type s struct {
			A *string    `db:"a"`
			B *int       `db:"b"`
			C *int8      `db:"c"`
			D *int16     `db:"d"`
			E *int32     `db:"e"`
			F *int64     `db:"f"`
			G *uint      `db:"g"`
			H *uint8     `db:"h"`
			I *uint16    `db:"i"`
			J *uint32    `db:"j"`
			K *uint64    `db:"k"`
			L *bool      `db:"l"`
			M *float32   `db:"m"`
			N *float64   `db:"n"`
			O *[]byte    `db:"o"`
			P *time.Time `db:"p"`
		}
		out := new(s)
		args := generateScanArgs(out)

		assert.Len(t, args, 16)
		now := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)

		t.Run("assign non-nulls", func(t *testing.T) {
			assert.NoError(t, args[0].(sql.Scanner).Scan("hello"))
			assert.NoError(t, args[1].(sql.Scanner).Scan(1))
			assert.NoError(t, args[2].(sql.Scanner).Scan(2))
			assert.NoError(t, args[3].(sql.Scanner).Scan(3))
			assert.NoError(t, args[4].(sql.Scanner).Scan(4))
			assert.NoError(t, args[5].(sql.Scanner).Scan(5))
			assert.NoError(t, args[6].(sql.Scanner).Scan(6))
			assert.NoError(t, args[7].(sql.Scanner).Scan(7))
			assert.NoError(t, args[8].(sql.Scanner).Scan(8))
			assert.NoError(t, args[9].(sql.Scanner).Scan(9))
			assert.NoError(t, args[10].(sql.Scanner).Scan(10))
			assert.NoError(t, args[11].(sql.Scanner).Scan(true))
			assert.NoError(t, args[12].(sql.Scanner).Scan(1.5))
			assert.NoError(t, args[13].(sql.Scanner).Scan(2.5))
			assert.NoError(t, args[14].(sql.Scanner).Scan([]byte("world")))
			assert.NoError(t, args[15].(sql.Scanner).Scan(now))

			assert.Equal(t, new("hello"), out.A)
			assert.Equal(t, new(1), out.B)
			assert.Equal(t, new(int8(2)), out.C)
			assert.Equal(t, new(int16(3)), out.D)
			assert.Equal(t, new(int32(4)), out.E)
			assert.Equal(t, new(int64(5)), out.F)
			assert.Equal(t, new(uint(6)), out.G)
			assert.Equal(t, new(uint8(7)), out.H)
			assert.Equal(t, new(uint16(8)), out.I)
			assert.Equal(t, new(uint32(9)), out.J)
			assert.Equal(t, new(uint64(10)), out.K)
			assert.Equal(t, new(true), out.L)
			assert.Equal(t, new(float32(1.5)), out.M)
			assert.Equal(t, new(2.5), out.N)
			assert.Equal(t, new([]byte("world")), out.O)
			assert.Equal(t, new(now), out.P)
		})
		t.Run("assign nulls", func(t *testing.T) {
			assert.NoError(t, args[0].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[1].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[2].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[3].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[4].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[5].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[6].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[7].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[8].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[9].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[10].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[11].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[12].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[13].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[14].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[15].(sql.Scanner).Scan(nil))

			assert.Nil(t, out.A)
			assert.Nil(t, out.B)
			assert.Nil(t, out.C)
			assert.Nil(t, out.D)
			assert.Nil(t, out.E)
			assert.Nil(t, out.F)
			assert.Nil(t, out.G)
			assert.Nil(t, out.H)
			assert.Nil(t, out.I)
			assert.Nil(t, out.J)
			assert.Nil(t, out.K)
			assert.Nil(t, out.L)
			assert.Nil(t, out.M)
			assert.Nil(t, out.N)
			assert.Nil(t, out.O)
			assert.Nil(t, out.P)
		})
	})
	t.Run("embedded structs", func(t *testing.T) {
		type inner struct {
			B string `db:"b"`
		}
		type outer struct {
			A string `db:"a"`
			inner
		}
		out := new(outer)
		args := generateScanArgs(out)

		assert.Len(t, args, 2)
		*args[0].(*string) = "first"
		*args[1].(*string) = "second"

		assert.Equal(t, "first", out.A)
		assert.Equal(t, "second", out.B)
	})
	t.Run("nested structs", func(t *testing.T) {
		type Inner2 struct {
			C string `db:"C"`
		}
		type Inner1 struct {
			B     string `db:"b"`
			Inner Inner2 `db:"inner"`
		}
		type Outer struct {
			A     string `db:"a"`
			Inner Inner1 `db:"inner"`
		}
		out := new(Outer)
		args := generateScanArgs(out)

		assert.Len(t, args, 3)
		*args[0].(*string) = "first"
		*args[1].(*string) = "second"
		*args[2].(*string) = "third"
		AssertAllComplete(args)

		assert.Equal(t, "first", out.A)
		assert.Equal(t, "second", out.Inner.B)
		assert.Equal(t, "third", out.Inner.Inner.C)
	})
	t.Run("nested nullable structs", func(t *testing.T) {
		type Inner2 struct {
			B string `db:"b"`
		}
		type Inner1 struct {
			A     string  `db:"a"`
			Inner *Inner2 `db:"inner"`
		}
		type Outer struct {
			Inner *Inner1 `db:"inner"`
		}

		t.Run("happy", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan("first"))
			assert.NoError(t, args[1].(sql.Scanner).Scan("second"))
			AssertAllComplete(args)

			assert.Equal(t, "first", out.Inner.A)
			assert.Equal(t, "second", out.Inner.Inner.B)
		})
		t.Run("both null, nothing happens", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[1].(sql.Scanner).Scan(nil))
			AssertAllComplete(args)

			assert.Nil(t, out.Inner)
		})
		t.Run("second null, only Inner1 allocates", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan("first"))
			assert.NoError(t, args[1].(sql.Scanner).Scan(nil))
			AssertAllComplete(args)

			assert.Equal(t, "first", out.Inner.A)
			assert.Nil(t, out.Inner.Inner)
		})
		t.Run("first null, both things allocate", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan(nil))
			assert.NoError(t, args[1].(sql.Scanner).Scan("second"))
			AssertAllComplete(args)

			assert.Equal(t, "", out.Inner.A)
			assert.Equal(t, "second", out.Inner.Inner.B)
		})
		t.Run("too many scans", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan("first"))
			assert.NoError(t, args[1].(sql.Scanner).Scan("second"))
			assert.Panics(t, func() {
				args[2].(sql.Scanner).Scan("third?!")
			})
		})
		t.Run("too few scans", func(t *testing.T) {
			out := new(Outer)
			args := generateScanArgs(out)

			require.Len(t, args, 2)
			assert.NoError(t, args[0].(sql.Scanner).Scan("first"))
			assert.Panics(t, func() {
				AssertAllComplete(args)
			})
		})
	})
}

func TestDBFields(t *testing.T) {
	type Inner1 struct {
		D int `db:"d"`
	}
	type Inner2 struct {
		E int `db:"e"`
	}
	type Inner4 struct {
		G int `db:"g"`
	}
	type Inner3 struct {
		F      int `db:"f"`
		Inner4 `db:"inner4"`
	}
	type Inner5 struct {
		H int `db:"h"`
	}
	type s struct {
		A      int    `db:"a"`
		B      int    // no tag, ignored
		c      int    `db:"c"` // not exported, ignored
		Inner1 Inner1 `db:"inner1"`
		Inner2 `db:"inner2"`
		Inner3 `db:"inner3"`
		Inner5 // no tag, contents still included
	}

	fields := slices.Collect(dbFields(reflect.TypeFor[s]()))
	t.Log(fields)
	require.Len(t, fields, 6)
	assert.Equal(t, "A", fields[0].Name)
	assert.Equal(t, "Inner1", fields[1].Name)
	assert.Equal(t, "E", fields[2].Name)
	assert.Equal(t, "F", fields[3].Name)
	assert.Equal(t, "G", fields[4].Name)
	assert.Equal(t, "H", fields[5].Name)
}

func TestCompileQuery(t *testing.T) {
	type Identifiable struct {
		ID string `db:"id"`
	}
	type Asset struct {
		Hash string `db:"hash"`
	}
	type User struct {
		Identifiable
		Name           string  `db:"name"`
		Nickname       *string `db:"nickname"`
		ProfilePicture Asset   `db:"profile_pic"`
		BannerImage    *Asset  `db:"banner_img"`
	}

	assert.Equal(t,
		len(generateScanArgs(&User{})),
		len(getColumnNames(reflect.TypeFor[User](), nil)),
	)
	assert.Equal(t,
		`SELECT id, name, nickname, profile_pic.hash, banner_img.hash FROM user`,
		compileQuery[User](`SELECT $columns FROM user`),
	)
	assert.Equal(t,
		`SELECT user.id, user.name, user.nickname, user_profile_pic.hash, user_banner_img.hash FROM user`,
		compileQuery[User](`SELECT $columns{user} FROM user`),
	)
}
