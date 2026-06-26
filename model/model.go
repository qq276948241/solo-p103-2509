package model

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

type GroupSession struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	CutoffTime  time.Time `json:"cutoff_time"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Product struct {
	ID        int64   `json:"id"`
	GroupID   int64   `json:"group_id"`
	Name      string  `json:"name"`
	UnitPrice float64 `json:"unit_price"`
	Unit      string  `json:"unit"`
	Stock     int     `json:"stock"`
	OnShelf   bool    `json:"on_shelf"`
	CreatedAt time.Time `json:"created_at"`
}

type Order struct {
	ID        int64     `json:"id"`
	GroupID   int64     `json:"group_id"`
	Phone     string    `json:"phone"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Remark    string    `json:"remark"`
	TotalAmt  float64   `json:"total_amt"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OrderItem struct {
	ID        int64   `json:"id"`
	OrderID   int64   `json:"order_id"`
	ProductID int64   `json:"product_id"`
	ProdName  string  `json:"prod_name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Subtotal  float64 `json:"subtotal"`
}

func InitDB(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	if err := createTables(); err != nil {
		return fmt.Errorf("create tables: %w", err)
	}

	if err := migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func migrate() error {
	rows, err := DB.Query("PRAGMA table_info(products)")
	if err != nil {
		return err
	}
	defer rows.Close()
	hasStock := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, dtype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &dtype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == "stock" {
			hasStock = true
			break
		}
	}
	if !hasStock {
		if _, err := DB.Exec("ALTER TABLE products ADD COLUMN stock INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	return nil
}

func createTables() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS group_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			cutoff_time DATETIME NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			created_at DATETIME NOT NULL DEFAULT (datetime('now','localtime'))
		)`,
		`CREATE TABLE IF NOT EXISTS products (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			unit_price REAL NOT NULL,
			unit TEXT NOT NULL DEFAULT '份',
			stock INTEGER NOT NULL DEFAULT 0,
			on_shelf INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT (datetime('now','localtime')),
			FOREIGN KEY (group_id) REFERENCES group_sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			phone TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '',
			total_amt REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT (datetime('now','localtime')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now','localtime')),
			FOREIGN KEY (group_id) REFERENCES group_sessions(id)
		)`,
		`CREATE TABLE IF NOT EXISTS order_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL,
			product_id INTEGER NOT NULL,
			prod_name TEXT NOT NULL,
			quantity INTEGER NOT NULL DEFAULT 1,
			unit_price REAL NOT NULL,
			subtotal REAL NOT NULL,
			FOREIGN KEY (order_id) REFERENCES orders(id),
			FOREIGN KEY (product_id) REFERENCES products(id)
		)`,
	}

	for _, s := range stmts {
		if _, err := DB.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
