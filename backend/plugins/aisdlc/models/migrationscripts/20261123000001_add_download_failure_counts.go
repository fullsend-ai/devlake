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
	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/migrationhelper"
)

// Frozen snapshot of the new checkpoint columns; do not migrate the runtime model.
type aisdlcCheckpointDownloadCounts20261123 struct {
	Id               string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId     uint64 `gorm:"primaryKey"`
	DownloadFailures int
	SummaryFiles     int
	SummaryFailures  int
}

func (aisdlcCheckpointDownloadCounts20261123) TableName() string {
	return "_tool_aisdlc_checkpoints"
}

type addDownloadFailureCounts struct{}

var _ plugin.MigrationScript = (*addDownloadFailureCounts)(nil)

func (*addDownloadFailureCounts) Up(res context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(res, &aisdlcCheckpointDownloadCounts20261123{})
}

func (*addDownloadFailureCounts) Version() uint64 { return 20261123000001 }
func (*addDownloadFailureCounts) Name() string    { return "aisdlc checkpoint download failure counts" }
