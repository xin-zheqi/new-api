package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// A small OOXML writer for a fixed report avoids an extra workbook dependency
// and writes exclusively to memory. Untrusted names are inline strings, never
// spreadsheet formulas, hyperlinks or external relationships.
func BuildTokenUsageWorkbook(rows []model.TokenUsageExportRow, start, end int64, chinese bool) ([]byte, error) {
	// Do not silently wrap aggregate totals or emit misleading negative usage.
	var totals [4]int64
	for _, row := range rows {
		for i, value := range []int64{row.Requests, row.PromptTokens, row.CompletionTokens, row.Quota} {
			if value < 0 || value > math.MaxInt64-totals[i] {
				return nil, errors.New("usage totals are out of range")
			}
			totals[i] += value
		}
	}
	if totals[1] > math.MaxInt64-totals[2] {
		return nil, errors.New("token total is out of range")
	}
	headers := []string{"API key name", "Model", "Requests", "Input tokens", "Output tokens", "Total tokens", "Amount (USD)", "Quota units", "First request (Beijing Time)", "Last request (Beijing Time)"}
	title := "API key usage report"
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	if chinese {
		title = "令牌模型用量报表"
		headers = []string{"令牌名称", "模型名称", "调用次数", "输入 Token", "输出 Token", "总 Token", "金额（美元）", "配额单位", "首次调用（北京时间）", "末次调用（北京时间）"}
	}
	date := func(value int64) string {
		if value == 0 {
			return "—"
		}
		return time.Unix(value, 0).In(beijing).Format("2006-01-02 15:04:05")
	}
	escape := func(value string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(strings.Map(func(r rune) rune {
			if r < 32 && r != '\n' && r != '\t' && r != '\r' {
				return -1
			}
			return r
		}, value)))
		return b.String()
	}
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0" showGridLines="0"><pane xSplit="2" ySplit="4" topLeftCell="C5" activePane="bottomRight" state="frozen"/></sheetView></sheetViews><cols><col min="1" max="2" width="28" customWidth="1"/><col min="3" max="6" width="18" customWidth="1"/><col min="7" max="8" width="22" customWidth="1"/><col min="9" max="10" width="25" customWidth="1"/></cols><sheetData>`)
	textCell := func(col, row, style int, value string) {
		fmt.Fprintf(&sheet, `<c r="%c%d" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, 'A'+col, row, style, escape(value))
	}
	for i, value := range []string{title, date(start) + " — " + date(end) + " (Beijing Time)"} {
		fmt.Fprintf(&sheet, `<row r="%d" ht="30" customHeight="1">`, i+1)
		style := 2
		if i == 0 {
			style = 1
		}
		textCell(0, i+1, style, value)
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`<row r="4" ht="30" customHeight="1">`)
	for col, value := range headers {
		textCell(col, 4, 3, value)
	}
	sheet.WriteString(`</row>`)
	merges := []string{"A1:J1", "A2:J2"}
	var totalRequests, totalInput, totalOutput, totalQuota int64
	groupStart, band := 5, 0
	for i, row := range rows {
		n := i + 5
		if i > 0 && row.TokenID != rows[i-1].TokenID {
			if groupStart < n-1 {
				merges = append(merges, fmt.Sprintf("A%d:A%d", groupStart, n-1))
			}
			groupStart = n
			band = 1 - band
		}
		fmt.Fprintf(&sheet, `<row r="%d" ht="28" customHeight="1">`, n)
		textCell(0, n, 4+band*3, row.TokenName)
		textCell(1, n, 4+band*3, row.ModelName)
		values := []string{strconv.FormatInt(row.Requests, 10), strconv.FormatInt(row.PromptTokens, 10), strconv.FormatInt(row.CompletionTokens, 10), strconv.FormatInt(row.PromptTokens+row.CompletionTokens, 10), strconv.FormatFloat(float64(row.Quota)/common.QuotaPerUnit, 'f', 8, 64), strconv.FormatInt(row.Quota, 10)}
		for j, v := range values {
			style := 5 + band*4
			if j >= 1 && j <= 3 {
				style = 7 + band*4
			} else if j == 4 {
				style = 6 + band*4
			}
			fmt.Fprintf(&sheet, `<c r="%c%d" s="%d"><v>%s</v></c>`, 'C'+j, n, style, v)
		}
		textCell(8, n, 4+band*4, date(row.FirstRequest))
		textCell(9, n, 4+band*4, date(row.LastRequest))
		sheet.WriteString(`</row>`)
		totalRequests += row.Requests
		totalInput += row.PromptTokens
		totalOutput += row.CompletionTokens
		totalQuota += row.Quota
	}
	if groupStart < len(rows)+4 {
		merges = append(merges, fmt.Sprintf("A%d:A%d", groupStart, len(rows)+4))
	}
	n := len(rows) + 5
	fmt.Fprintf(&sheet, `<row r="%d" ht="30" customHeight="1">`, n)
	totalLabel := "Total"
	if chinese {
		totalLabel = "合计"
	}
	textCell(0, n, 3, totalLabel)
	for j, v := range []string{fmt.Sprint(totalRequests), fmt.Sprint(totalInput), fmt.Sprint(totalOutput), fmt.Sprint(totalInput + totalOutput), strconv.FormatFloat(float64(totalQuota)/common.QuotaPerUnit, 'f', 8, 64), fmt.Sprint(totalQuota)} {
		style := 5
		if j >= 1 && j <= 3 {
			style = 7
		} else if j == 4 {
			style = 6
		}
		fmt.Fprintf(&sheet, `<c r="%c%d" s="%d"><v>%s</v></c>`, 'C'+j, n, style, v)
	}
	sheet.WriteString(`</row></sheetData>`)
	fmt.Fprintf(&sheet, `<autoFilter ref="B4:J%d"/><mergeCells count="%d">`, max(4, len(rows)+4), len(merges))
	for _, ref := range merges {
		fmt.Fprintf(&sheet, `<mergeCell ref="%s"/>`, ref)
	}
	sheet.WriteString(`</mergeCells><pageSetup orientation="landscape" paperSize="9" fitToWidth="1" fitToHeight="0"/></worksheet>`)
	var styles strings.Builder
	styles.WriteString(`<?xml version="1.0" encoding="UTF-8"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><numFmts count="2"><numFmt numFmtId="164" formatCode="#,##0.00000000"/><numFmt numFmtId="165" formatCode="[&gt;=1000000]0.##,,&quot;M&quot;;[&gt;=1000]0.##,&quot;K&quot;;0"/></numFmts><fonts count="3"><font><sz val="11"/><name val="SimSun"/><color rgb="FF243247"/></font><font><sz val="20"/><b/><name val="SimSun"/><color rgb="FF18344A"/></font><font><sz val="11"/><b/><name val="SimSun"/><color rgb="FFFFFFFF"/></font></fonts><fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF18344A"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFF0F5F9"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="12"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/><xf fontId="1" applyFont="1"><alignment vertical="center"/></xf><xf fontId="0"><alignment vertical="center" wrapText="1"/></xf><xf fontId="2" fillId="2" applyFont="1" applyFill="1"><alignment vertical="center" horizontal="center" wrapText="1"/></xf>`)
	for _, fill := range []int{0, 3} {
		for _, format := range []int{0, 3, 164, 165} {
			align := "right"
			if format == 0 {
				align = "center"
			}
			fmt.Fprintf(&styles, `<xf fontId="0" fillId="%d" numFmtId="%d" applyFill="1" applyNumberFormat="1"><alignment horizontal="%s" vertical="center" wrapText="1"/></xf>`, fill, format, align)
		}
	}
	styles.WriteString(`</cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`)
	files := []struct{ name, content string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Usage" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", styles.String()}, {"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var buffer bytes.Buffer
	workbook := zip.NewWriter(&buffer)
	for _, file := range files {
		writer, err := workbook.Create(file.name)
		if err != nil {
			return nil, err
		}
		if _, err = writer.Write([]byte(file.content)); err != nil {
			return nil, err
		}
	}
	if err := workbook.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
