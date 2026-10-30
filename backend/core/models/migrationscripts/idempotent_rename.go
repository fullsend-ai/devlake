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
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
)

// renameTableIfAbsent renames oldName to newName when the source table exists and the target does not.
// Plugin e2e tests AutoMigrate final table names on the shared database without recording migrations.
func renameTableIfAbsent(db dal.Dal, oldName, newName string) errors.Error {
	if db.HasTable(oldName) && !db.HasTable(newName) {
		return db.RenameTable(oldName, newName)
	}
	return nil
}

// renameColumnIfAbsent renames a column when the old name exists and the new name does not.
func renameColumnIfAbsent(db dal.Dal, table, oldColumn, newColumn string) errors.Error {
	if !db.HasTable(table) {
		return nil
	}
	if db.HasColumn(table, oldColumn) && !db.HasColumn(table, newColumn) {
		return db.RenameColumn(table, oldColumn, newColumn)
	}
	return nil
}

// dropColumnsIfExist drops columns that are still present on the table.
func dropColumnsIfExist(db dal.Dal, table string, columnNames ...string) errors.Error {
	if !db.HasTable(table) {
		return nil
	}
	toDrop := make([]string, 0, len(columnNames))
	for _, columnName := range columnNames {
		if db.HasColumn(table, columnName) {
			toDrop = append(toDrop, columnName)
		}
	}
	if len(toDrop) == 0 {
		return nil
	}
	return db.DropColumns(table, toDrop...)
}
