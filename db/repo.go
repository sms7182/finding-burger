package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrNoRows = errors.New("vendor not found")

type Postgres struct {
	db *gorm.DB
}

type Vendor struct {
	ID          uint `gorm:"primaryKey"`
	Name        string
	BaseLoad    int
	CurrentLoad int
}

func (Vendor) TableName() string { return "vendors" }

type EligibleVendor struct {
	ID       int    `gorm:"column:id"`
	Name     string `gorm:"column:name"`
	BaseLoad int    `gorm:"column:base_load"`
	DBLoad   int    `gorm:"column:db_load"`
}

type VendorRow struct {
	ID       int    `gorm:"column:id"`
	Name     string `gorm:"column:name"`
	ZoneJSON string `gorm:"column:zone_json"`
	BaseLoad int    `gorm:"column:base_load"`
}

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	var gdb *gorm.DB
	var err error

	deadline := time.Now().Add(90 * time.Second)
	for {
		gdb, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err == nil {
			if raw, derr := gdb.DB(); derr == nil {
				pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				perr := raw.PingContext(pingCtx)
				cancel()
				if perr == nil {
					break
				}
				err = perr
			} else {
				err = derr
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("postgres not reachable: %w", err)
		}
		log.Printf("postgres not ready yet, retrying: %v", err)
		time.Sleep(2 * time.Second)
	}

	if sqlDB, e := gdb.DB(); e == nil {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	return &Postgres{db: gdb}, nil
}

func (p *Postgres) Close() {
	if sqlDB, err := p.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

func (p *Postgres) EligibleVendors(ctx context.Context, lat, lon float64) ([]EligibleVendor, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var out []EligibleVendor
	err := p.db.WithContext(ctx).Raw(`
		SELECT id, name, base_load, current_load AS db_load
		FROM vendors
		WHERE ST_Contains(
			service_zone,
			ST_SetSRID(ST_MakePoint(?, ?), 4326)
		)
		ORDER BY id`, lon, lat).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("eligibility query: %w", err)
	}
	return out, nil
}

func (p *Postgres) ListVendors(ctx context.Context) ([]VendorRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var out []VendorRow
	err := p.db.WithContext(ctx).Raw(`
		SELECT id, name, ST_AsGeoJSON(service_zone) AS zone_json, base_load
		FROM vendors
		ORDER BY id`).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list vendors: %w", err)
	}
	return out, nil
}

func (p *Postgres) UpdateZone(ctx context.Context, id int, geojson []byte) (VendorRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var v VendorRow
	res := p.db.WithContext(ctx).Raw(`
		UPDATE vendors
		SET service_zone = ST_SetSRID(ST_GeomFromGeoJSON(?), 4326),
		    updated_at   = now()
		WHERE id = ?
		RETURNING id, name, ST_AsGeoJSON(service_zone) AS zone_json, base_load`,
		string(geojson), id).Scan(&v)
	if res.Error != nil {
		return VendorRow{}, res.Error
	}
	if res.RowsAffected == 0 {
		return VendorRow{}, ErrNoRows
	}
	return v, nil
}

func (p *Postgres) SetLoad(ctx context.Context, id, load int) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.db.WithContext(ctx).
		Model(&Vendor{}).
		Where("id = ?", id).
		Update("current_load", load).Error
}
