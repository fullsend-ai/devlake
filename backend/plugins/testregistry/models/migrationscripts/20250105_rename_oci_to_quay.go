/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package migrationscripts

import (
	"fmt"
	"strings"

	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
)

var _ plugin.MigrationScript = (*renameOciToQuay)(nil)

type renameOciToQuay struct{}

func (*renameOciToQuay) Up(basicRes context.BasicRes) errors.Error {
	db := basicRes.GetDal()

	if err := renameOrAddColumn(db, "_tool_testregistry_connections", "oci_artifact_url", "quay_organization", "VARCHAR(200)"); err != nil {
		return err
	}
	if err := renameOrAddColumn(db, "_tool_testregistry_scopes", "repository_full_name", "full_name", "VARCHAR(500)"); err != nil {
		return err
	}
	if err := renameOrAddColumn(db, "_tool_testregistry_scopes", "repository_name", "name", "VARCHAR(500)"); err != nil {
		return err
	}

	// Copy a legacy scope column when it is still present, then drop it.
	_ = db.Exec("UPDATE _tool_testregistry_scopes SET full_name = oci_artifact_url WHERE (full_name IS NULL OR full_name = '') AND oci_artifact_url IS NOT NULL AND oci_artifact_url != ''")
	_ = db.Exec("ALTER TABLE _tool_testregistry_scopes DROP COLUMN oci_artifact_url")

	return nil
}

// renameOrAddColumn renames oldName to newName, or adds newName when the old
// column is already gone. MySQL uses CHANGE COLUMN; Postgres uses RENAME COLUMN.
func renameOrAddColumn(db dal.Dal, table, oldName, newName, colType string) errors.Error {
	if db.HasColumn(table, oldName) {
		var stmt string
		if db.Dialect() == "postgres" {
			stmt = fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", table, oldName, newName)
		} else {
			stmt = fmt.Sprintf("ALTER TABLE %s CHANGE COLUMN %s %s %s", table, oldName, newName, colType)
		}
		if err := db.Exec(stmt); err != nil {
			return errors.Default.Wrap(err, "rename "+oldName+" to "+newName)
		}
		return nil
	}
	if db.HasColumn(table, newName) {
		return nil
	}
	if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, newName, colType)); err != nil && !columnAlreadyPresent(err) {
		return errors.Default.Wrap(err, "add "+newName)
	}
	return nil
}

// columnAlreadyPresent reports a duplicate-column error from MySQL or Postgres.
func columnAlreadyPresent(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate column name") ||
		strings.Contains(msg, "1060") ||
		strings.Contains(msg, "already exists")
}

func (*renameOciToQuay) Version() uint64 {
	return 20250105000001
}

func (*renameOciToQuay) Name() string {
	return "rename oci_artifact_url to quay_organization and update scope fields"
}
