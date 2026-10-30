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
)

var _ plugin.MigrationScript = (*renameDeploymentIdForPrProjectMetric)(nil)

type renameDeploymentIdForPrProjectMetric struct{}

func (*renameDeploymentIdForPrProjectMetric) Up(basicRes context.BasicRes) errors.Error {
	db := basicRes.GetDal()
	if !db.HasTable("project_pr_metrics") {
		return nil
	}
	hasOld := db.HasColumn("project_pr_metrics", "deployment_id")
	hasNew := db.HasColumn("project_pr_metrics", "deployment_commit_id")
	if hasOld && !hasNew {
		return renameColumnIfAbsent(db, "project_pr_metrics", "deployment_id", "deployment_commit_id")
	}
	if hasOld && hasNew {
		return dropColumnsIfExist(db, "project_pr_metrics", "deployment_id")
	}
	return nil
}

func (*renameDeploymentIdForPrProjectMetric) Version() uint64 {
	return 20230416080701
}

func (*renameDeploymentIdForPrProjectMetric) Name() string {
	return "Rename project_pr_metrics.deployment_id to deployment_commit_id"
}
