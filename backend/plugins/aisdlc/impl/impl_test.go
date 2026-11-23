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

package impl

import (
	"testing"

	"github.com/apache/devlake/helpers/unithelper"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/stretchr/testify/require"
)

func TestUsageScopeConfigurations(t *testing.T) {
	for _, tt := range []struct {
		parser, mode string
		valid        bool
	}{
		{"rfe_assess", "", true}, {"strat_pipeline", "off", true},
		{"strat_pipeline", "summary", true}, {"", "summary", true},
		{"", "", false}, {"", "off", false}, {"unknown", "summary", false},
		{"rfe_assess", "raw", false},
	} {
		cfg := &models.AisdlcScopeConfig{TenantId: "tenant", SourceId: "source", StationId: "station", Parser: tt.parser, UsageMode: tt.mode}
		require.Equal(t, tt.valid, validateScopeConfig(cfg) == nil, "%+v", tt)
	}
}

func TestUsageTablesRegistered(t *testing.T) {
	checker := unithelper.NewTableInfoChecker(unithelper.TableInfoCheckerConfig{})
	checker.FeedIn("../models", Aisdlc{}.GetTablesInfo)
	require.Nil(t, checker.Verify())
}
