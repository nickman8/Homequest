// Applies the exact bot migrations without contacting Telegram or starting workers.
package main

import (
	"donetick.com/core/internal/homequest/bot"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"syscall"
)

func main() {
	if !migrate() {
		fmt.Fprintln(os.Stderr, "Migration failed: check DATA_DIR, database permissions and stop the polling process first.")
		os.Exit(1)
	}
	fmt.Println("HomeQuest migrations complete")
}
func migrate() bool {
	dir := os.Getenv("DATA_DIR")
	if dir == "" {
		return false
	}
	if os.MkdirAll(dir, 0700) != nil {
		return false
	}
	lock, e := os.OpenFile(filepath.Join(dir, "polling.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return false
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	db, e := gorm.Open(sqlite.Open(filepath.Join(dir, "donetick.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		return false
	}
	sql, e := db.DB()
	if e != nil {
		return false
	}
	defer sql.Close()
	sql.SetMaxOpenConns(1)
	_, e = bot.New(db, "", "")
	return e == nil
}
