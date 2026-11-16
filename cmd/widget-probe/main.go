// Controlled transport for the compatibility harness. No Telegram polling.
package main

import (
	"donetick.com/core/internal/homequest/widget"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	db, e := gorm.Open(sqlite.Open(os.Args[1]), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		os.Exit(1)
	}
	s, e := widget.NewSpike(db)
	if e != nil {
		os.Exit(1)
	}
	var req struct {
		Viewer int64  `json:"viewer"`
		Action string `json:"action"`
	}
	if json.NewDecoder(os.Stdin).Decode(&req) != nil {
		os.Exit(2)
	}
	var state widget.State
	if req.Action == "" {
		state, e = s.Issue(req.Viewer, time.Now())
	} else {
		state, e = s.Act(req.Viewer, req.Action, time.Now())
	}
	if e != nil {
		fmt.Println(`{"error":"action unavailable"}`)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(state)
}
