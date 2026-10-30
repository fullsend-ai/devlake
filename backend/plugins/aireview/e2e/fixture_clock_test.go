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

package e2e

import (
	"testing"
	"time"

	"github.com/apache/devlake/core/dal"
)

// ciFixtureAnchor is the newest finished_at in the checked-in CI fixtures
// (the periodic flaky job). Prediction and backfill queries use rolling
// windows, so the fixtures are shifted until this instant is five days ago.
const ciFixtureAnchor = "2026-04-15T02:00:00Z"

// anchorCIFixtures moves imported pull_requests and ci_test_jobs timestamps
// forward by a fixed offset. Relative gaps stay the same, so the flaky job
// remains inside the 30-day window and the March PR rows remain inside the
// 90-day backfill and 3-month outcome windows.
func anchorCIFixtures(t *testing.T, d dal.Dal) {
	t.Helper()
	anchor, err := time.Parse(time.RFC3339, ciFixtureAnchor)
	if err != nil {
		t.Fatalf("parse fixture anchor: %v", err)
	}
	target := time.Now().UTC().AddDate(0, 0, -5)
	delta := target.Sub(anchor)
	if delta <= 0 {
		return
	}
	shiftJobTimes(t, d, delta)
	shiftPullRequestTimes(t, d, delta)
}

type jobClockRow struct {
	ConnectionId int64  `gorm:"column:connection_id"`
	JobId        string `gorm:"column:job_id"`
	FinishedAt   string `gorm:"column:finished_at"`
}

func (jobClockRow) TableName() string { return "ci_test_jobs" }

func shiftJobTimes(t *testing.T, d dal.Dal, delta time.Duration) {
	t.Helper()
	var rows []jobClockRow
	if err := d.All(&rows); err != nil {
		t.Fatalf("load ci_test_jobs timestamps: %v", err)
	}
	for _, row := range rows {
		if row.FinishedAt == "" {
			continue
		}
		ts, err := parseFixtureTime(row.FinishedAt)
		if err != nil {
			t.Fatalf("parse finished_at %q: %v", row.FinishedAt, err)
		}
		shifted := ts.Add(delta).UTC().Format("2006-01-02 15:04:05")
		if err := d.Exec(
			"UPDATE ci_test_jobs SET finished_at = ? WHERE connection_id = ? AND job_id = ?",
			shifted, row.ConnectionId, row.JobId,
		); err != nil {
			t.Fatalf("shift ci_test_jobs.finished_at: %v", err)
		}
	}
}

type prClockRow struct {
	Id          string    `gorm:"column:id"`
	CreatedDate time.Time `gorm:"column:created_date"`
	MergedDate  time.Time `gorm:"column:merged_date"`
}

func (prClockRow) TableName() string { return "pull_requests" }

func shiftPullRequestTimes(t *testing.T, d dal.Dal, delta time.Duration) {
	t.Helper()
	var rows []prClockRow
	if err := d.All(&rows); err != nil {
		t.Fatalf("load pull_requests timestamps: %v", err)
	}
	for _, row := range rows {
		if !row.CreatedDate.IsZero() {
			if err := d.Exec(
				"UPDATE pull_requests SET created_date = ? WHERE id = ?",
				row.CreatedDate.Add(delta), row.Id,
			); err != nil {
				t.Fatalf("shift pull_requests.created_date: %v", err)
			}
		}
		if !row.MergedDate.IsZero() {
			if err := d.Exec(
				"UPDATE pull_requests SET merged_date = ? WHERE id = ?",
				row.MergedDate.Add(delta), row.Id,
			); err != nil {
				t.Fatalf("shift pull_requests.merged_date: %v", err)
			}
		}
	}
}

func parseFixtureTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
	}
	var last error
	for _, layout := range layouts {
		ts, err := time.Parse(layout, value)
		if err == nil {
			return ts, nil
		}
		last = err
	}
	return time.Time{}, last
}
