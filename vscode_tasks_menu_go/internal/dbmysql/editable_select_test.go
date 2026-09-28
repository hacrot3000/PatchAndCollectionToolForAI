package dbmysql

import "testing"

func TestAnalyzeEditableSelectAcceptsSingleTableDirectColumns(t *testing.T) {
	cases := []struct {
		statement string
		catalog   string
		table     string
		alias     string
		all       bool
		columns   []string
	}{
		{"SELECT * FROM activity", "", "activity", "", true, nil},
		{"SELECT id, name, status FROM activity WHERE status = 1 ORDER BY id DESC LIMIT 20", "", "activity", "", false, []string{"id", "name", "status"}},
		{"SELECT a.id, a.name FROM main.activity AS a WHERE a.id > 10", "main", "activity", "a", false, []string{"id", "name"}},
		{"SELECT activity.* FROM activity", "", "activity", "", true, nil},
	}
	for _, tc := range cases {
		got, err := analyzeEditableSelect(tc.statement)
		if err != nil {
			t.Fatalf("%q: %v", tc.statement, err)
		}
		if got.Catalog != tc.catalog || got.Table != tc.table || got.Alias != tc.alias || got.SelectAll != tc.all {
			t.Fatalf("%q target=%+v", tc.statement, got)
		}
		if len(got.Columns) != len(tc.columns) {
			t.Fatalf("%q columns=%v want %v", tc.statement, got.Columns, tc.columns)
		}
		for i := range got.Columns {
			if got.Columns[i] != tc.columns[i] {
				t.Fatalf("%q columns=%v want %v", tc.statement, got.Columns, tc.columns)
			}
		}
	}
}

func TestAnalyzeEditableSelectRejectsAmbiguousOrDerivedResults(t *testing.T) {
	for _, statement := range []string{
		"SELECT DISTINCT id, name FROM activity",
		"SELECT id AS activity_id, name FROM activity",
		"SELECT id, CONCAT(first_name, last_name) FROM activity",
		"SELECT a.id, b.name FROM activity a JOIN users b ON b.id = a.user_id",
		"SELECT id, COUNT(*) FROM activity GROUP BY id",
		"SELECT id FROM activity UNION SELECT id FROM archive_activity",
		"SELECT id, * FROM activity",
		"SELECT other.id FROM activity",
	} {
		if _, err := analyzeEditableSelect(statement); err == nil {
			t.Fatalf("%q unexpectedly editable", statement)
		}
	}
}
