package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageWorkbookMergedNamesNumericAmountsAndEscaping(t *testing.T) {
	rows := []model.TokenUsageExportRow{
		{TokenID: 1, TokenName: "=HYPERLINK(\"bad\") & <Agent>", ModelName: "model-a", Requests: 2, PromptTokens: 3000, CompletionTokens: 5, Quota: 500000, FirstRequest: 100, LastRequest: 200},
		{TokenID: 1, TokenName: "=HYPERLINK(\"bad\") & <Agent>", ModelName: "model-b", Requests: 1, PromptTokens: 4, CompletionTokens: 1, Quota: 250000, FirstRequest: 150, LastRequest: 150},
	}
	workbook, err := BuildTokenUsageWorkbook(rows, 100, 200, true)
	require.NoError(t, err)
	archive, err := zip.NewReader(bytes.NewReader(workbook), int64(len(workbook)))
	require.NoError(t, err)
	var sheet, styles string
	for _, file := range archive.File {
		reader, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		decoder := xml.NewDecoder(bytes.NewReader(body))
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		if file.Name == "xl/worksheets/sheet1.xml" {
			sheet = string(body)
		}
		if file.Name == "xl/styles.xml" {
			styles = string(body)
		}
		assert.NotContains(t, string(body), "<f>", "untrusted names must never become formulas")
		assert.NotContains(t, string(body), "TargetMode=\"External\"")
	}
	assert.Contains(t, sheet, `<mergeCell ref="A5:A6"/>`)
	assert.Contains(t, sheet, "1970-01-01 08:01:40")
	assert.Contains(t, sheet, "1970-01-01 08:03:20")
	assert.Contains(t, sheet, `<c r="G5" s="6"><v>1.00000000</v></c>`)
	assert.Contains(t, sheet, `<c r="G7" s="6"><v>1.50000000</v></c>`)
	assert.Contains(t, sheet, `<c r="D5" s="7"><v>3000</v></c>`)
	assert.Contains(t, sheet, `<c r="E5" s="7"><v>5</v></c>`)
	assert.Contains(t, sheet, `<c r="F5" s="7"><v>3005</v></c>`)
	assert.Contains(t, styles, `numFmtId="165"`)
	assert.Contains(t, styles, `formatCode="[&gt;=1000000]0.##,,&quot;M&quot;;[&gt;=1000]0.##,&quot;K&quot;;0"`)
	assert.Contains(t, styles, `name val="SimSun"`)
	assert.NotContains(t, sheet, "仅统计消费日志")
	assert.Contains(t, sheet, "&amp; &lt;Agent&gt;")
	assert.Contains(t, sheet, `state="frozen"`)
}

func TestTokenUsageWorkbookRejectsOverflowAndHandlesEmptySelection(t *testing.T) {
	_, err := BuildTokenUsageWorkbook([]model.TokenUsageExportRow{{PromptTokens: math.MaxInt64, CompletionTokens: 1}}, 0, 100, false)
	require.Error(t, err)
	workbook, err := BuildTokenUsageWorkbook(nil, 0, 100, false)
	require.NoError(t, err)
	assert.NotEmpty(t, workbook)
}
