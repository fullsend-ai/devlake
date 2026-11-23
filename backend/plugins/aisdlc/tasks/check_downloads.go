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

package tasks

import (
	"fmt"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/plugins/aisdlc/models"
)

// CheckDownloads runs last so valid artifacts are extracted and linked before
// a widespread GitLab download failure marks the pipeline as failed.
var CheckDownloadsMeta = plugin.SubTaskMeta{
	Name:             "checkDownloads",
	EntryPoint:       CheckDownloads,
	EnabledByDefault: true,
	Description:      "Fail collection if no selected artifacts or usage summaries could be downloaded",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CODE},
	Dependencies:     []*plugin.SubTaskMeta{&LinkWorkItemsMeta, &ExtractUsageMeta},
}

func CheckDownloads(taskCtx plugin.SubTaskContext) errors.Error {
	if policy := taskCtx.TaskContext().SyncPolicy(); policy != nil && policy.SkipCollectors {
		return nil
	}
	data := taskCtx.GetData().(*AisdlcTaskData)
	checkpoint := &models.AisdlcCheckpoint{}
	if err := taskCtx.GetDal().First(checkpoint, dal.Where(
		"id = ? AND connection_id = ?", checkpointID(data.Connection.ID, data.Options.FullName), data.Connection.ID,
	)); err != nil {
		return err
	}
	if checkpoint.FileCount > 0 && checkpoint.DownloadFailures == checkpoint.FileCount {
		return errors.Default.New(fmt.Sprintf("all %d AISDLC artifact downloads failed for %s; see checkpoint and logs",
			checkpoint.FileCount, data.Options.FullName))
	}
	resultFiles := checkpoint.FileCount - checkpoint.SummaryFiles
	resultFailures := checkpoint.DownloadFailures - checkpoint.SummaryFailures
	if resultFiles > 0 && resultFailures == resultFiles {
		return errors.Default.New(fmt.Sprintf("all %d AISDLC result downloads failed for %s; see checkpoint and logs",
			resultFiles, data.Options.FullName))
	}
	if data.Options.ScopeConfig.UsageEnabled() && checkpoint.SummaryFiles > 0 &&
		checkpoint.SummaryFailures == checkpoint.SummaryFiles {
		return errors.Default.New(fmt.Sprintf("all %d AISDLC usage summary downloads failed for %s; see checkpoint and logs",
			checkpoint.SummaryFiles, data.Options.FullName))
	}
	return nil
}
