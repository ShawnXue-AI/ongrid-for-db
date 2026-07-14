package store

import (
	"gorm.io/gorm"

	model "github.com/ongridio/ongrid/internal/manager/model/database"
)

// Migrate registers the database instance model with gorm's AutoMigrate
// and handles the index migration from (edge_id, name) to (edge_id, source_id).
func Migrate(db *gorm.DB) error {
	// Phase 1: drop the old unique index on (edge_id, name) if it exists.
	// GORM created it with the name "uk_edge_name" on MySQL and
	// "idx_database_instances_edge_id_name" on SQLite. We try both names
	// and ignore "not found" errors.
	_ = db.Migrator().DropIndex(&model.DatabaseInstance{}, "uk_edge_name")
	_ = db.Migrator().DropIndex(&model.DatabaseInstance{}, "idx_database_instances_edge_id_name")

	// Phase 2: ensure the new columns (source_id, plugin_type) exist BEFORE
	// the backfill, so we can write into them. We must NOT run AutoMigrate
	// here because it would also create the unique index uk_edge_source,
	// which would fail if any edge has multiple rows with source_id=''.
	if !db.Migrator().HasColumn(&model.DatabaseInstance{}, "source_id") {
		if err := db.Migrator().AddColumn(&model.DatabaseInstance{}, "SourceID"); err != nil {
			return err
		}
	}
	if !db.Migrator().HasColumn(&model.DatabaseInstance{}, "plugin_type") {
		if err := db.Migrator().AddColumn(&model.DatabaseInstance{}, "PluginType"); err != nil {
			return err
		}
	}

	// Phase 3: backfill source_id = name for existing rows that have an
	// empty source_id. This ensures the new unique constraint is satisfied
	// for rows created before the migration.
	_ = db.Exec(`UPDATE database_instances SET source_id = name WHERE source_id = '' AND name != ''`).Error

	// Phase 4: let AutoMigrate create the new unique index (edge_id, source_id).
	// At this point source_id is populated for all existing rows, so no
	// duplicate-key collisions should occur.
	return db.AutoMigrate(&model.DatabaseInstance{})
}
