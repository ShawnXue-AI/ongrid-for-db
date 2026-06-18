package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ongridio/ongrid/internal/pkg/tunnel"
)

const ToolNameInspectSchema = "inspect_schema"

const InspectSchemaDescription = `Inspect database table schemas via SHOW CREATE TABLE (MySQL) or equivalent.
Returns the DDL for analysis — missing indexes, charset mismatches,
auto-increment overflow risk, foreign key issues, column type review.
If no table_name is provided, lists all tables in the database first.`

const inspectSchemaWhenToUse = `When the user asks about table structure, schema design, or needs to review DDL:
- "Show me the schema of the users table"
- "Check if there are any tables without primary keys"
- "Are there any charset mismatches in this database?"
- "Check auto_increment values approaching max int"
- "Review foreign key relationships"
Use together with query_database for deeper analysis.`

var InspectSchemaSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "database_id": {
      "type": "integer",
      "description": "Database instance ID (set by ongrid instance management)"
    },
    "edge_id": {
      "type": "integer",
      "description": "Edge agent ID that hosts the database"
    },
    "db_type": {
      "type": "string",
      "enum": ["mysql", "postgresql", "selectdb"],
      "description": "Database type"
    },
    "host": {
      "type": "string",
      "description": "Database host reachable from the edge"
    },
    "port": {
      "type": "integer",
      "description": "Database port"
    },
    "database": {
      "type": "string",
      "description": "Database/schema to inspect"
    },
    "table_name": {
      "type": "string",
      "description": "Specific table to inspect (omit to list all tables)"
    }
  },
  "required": ["database"]
}`)

// executeInspectSchema retrieves table schemas from a database via the edge.
// Without table_name: lists all tables. With table_name: returns SHOW CREATE TABLE.
func (r *Registry) executeInspectSchema(ctx context.Context, args json.RawMessage) (ExecuteResult, error) {
	if r.caller == nil {
		return ExecuteResult{}, fmt.Errorf("%s: tunnel caller not configured", ToolNameInspectSchema)
	}

	var in struct {
		DatabaseID uint64 `json:"database_id,omitempty"`
		EdgeID    uint64 `json:"edge_id,omitempty"`
		DBType    string `json:"db_type"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user,omitempty"`
		Password  string `json:"password,omitempty"`
		Database  string `json:"database"`
		TableName string `json:"table_name,omitempty"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return ExecuteResult{}, fmt.Errorf("%s: bad args: %w", ToolNameInspectSchema, err)
	}
	if in.Database == "" {
		return ExecuteResult{}, fmt.Errorf("%s: database required", ToolNameInspectSchema)
	}

			// Resolve credentials server-side if not provided by the caller.
		// This keeps database passwords out of the LLM prompt context.
		if (in.User == `` || in.Password == ``) && in.DatabaseID > 0 && r.credentialResolver != nil {
			user, pass, found, err := r.credentialResolver.LookupCredentials(ctx, in.DatabaseID)
			if err != nil {
				return ExecuteResult{}, fmt.Errorf(`%s: resolve credentials: %w`, ToolNameInspectSchema, err)
			}
			if found {
				if in.User == `` { in.User = user }
				if in.Password == `` { in.Password = pass }
			}
		}
		if in.User == `` || in.Password == `` {
			return ExecuteResult{}, fmt.Errorf(`%s: user and password are required (provide database_id for server-side resolution, or pass credentials directly)`, ToolNameInspectSchema)
		}

// Helper to run a query via db_exec_query on the edge.