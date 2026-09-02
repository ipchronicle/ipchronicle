package notifications

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipchronicle/ipchronicle/internal/probefields"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	telegramImageWidth  = 1080
	telegramImageMargin = 60
	maximumImageChanges = 10
)

//go:embed assets/NotoSansCJKsc-Regular.otf
var telegramFontData []byte

var (
	telegramFontOnce sync.Once
	telegramFontSet  imageFonts
	telegramFontErr  error
	telegramRenderMu sync.Mutex
)

type imageTone uint8

const (
	toneNeutral imageTone = iota
	toneGreen
	toneRed
	toneAmber
	toneBlue
)

type imagePalette struct {
	background color.RGBA
	text       color.RGBA
	muted      color.RGBA
	border     color.RGBA
	green      color.RGBA
	greenSoft  color.RGBA
	greenEdge  color.RGBA
	red        color.RGBA
	redSoft    color.RGBA
	redEdge    color.RGBA
	amber      color.RGBA
	amberSoft  color.RGBA
	amberEdge  color.RGBA
	blue       color.RGBA
	blueSoft   color.RGBA
	blueEdge   color.RGBA
}

var telegramPalette = imagePalette{
	background: rgba(9, 9, 11),
	text:       rgba(250, 250, 250),
	muted:      rgba(161, 161, 170),
	border:     rgba(63, 63, 70),
	green:      rgba(52, 211, 153),
	greenSoft:  rgba(6, 46, 35),
	greenEdge:  rgba(6, 95, 70),
	red:        rgba(248, 113, 113),
	redSoft:    rgba(69, 10, 10),
	redEdge:    rgba(153, 27, 27),
	amber:      rgba(251, 191, 36),
	amberSoft:  rgba(66, 32, 6),
	amberEdge:  rgba(146, 64, 14),
	blue:       rgba(96, 165, 250),
	blueSoft:   rgba(23, 37, 84),
	blueEdge:   rgba(30, 64, 175),
}

type imageFonts struct {
	title      font.Face
	badge      font.Face
	meta       font.Face
	ip         font.Face
	ipCompact  font.Face
	label      font.Face
	source     font.Face
	value      font.Face
	valueSmall font.Face
	arrow      font.Face
	status     font.Face
}

type imageChange struct {
	label      string
	source     string
	before     string
	beforeTone imageTone
	after      string
	afterTone  imageTone
}

type imageFact struct {
	label string
	value string
	tone  imageTone
}

type imageContent struct {
	title      string
	badge      string
	badgeTone  imageTone
	meta       string
	address    string
	family     string
	status     string
	statusTone imageTone
	changes    []imageChange
	facts      []imageFact
	remaining  int
	locale     string
}

func renderTelegramImage(eventJSON []byte) ([]byte, error) {
	telegramRenderMu.Lock()
	defer telegramRenderMu.Unlock()
	var event eventEnvelope
	if len(eventJSON) == 0 || json.Unmarshal(eventJSON, &event) != nil || event.Type == "" {
		return nil, errors.New("decode Telegram notification event")
	}

	fonts, err := loadTelegramFonts()
	if err != nil {
		return nil, err
	}
	content := buildTelegramImageContent(event)
	height := telegramImageHeight(content)
	canvas := image.NewRGBA(image.Rect(0, 0, telegramImageWidth, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(telegramPalette.background), image.Point{}, draw.Src)
	drawTelegramImage(canvas, fonts, content)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return nil, fmt.Errorf("encode Telegram notification image: %w", err)
	}
	return encoded.Bytes(), nil
}

func loadTelegramFonts() (imageFonts, error) {
	telegramFontOnce.Do(func() {
		parsed, err := opentype.Parse(telegramFontData)
		if err != nil {
			telegramFontErr = fmt.Errorf("parse embedded notification font: %w", err)
			return
		}
		newFace := func(size float64) font.Face {
			face, faceErr := opentype.NewFace(parsed, &opentype.FaceOptions{
				Size: size, DPI: 72, Hinting: font.HintingFull,
			})
			if faceErr != nil && telegramFontErr == nil {
				telegramFontErr = faceErr
			}
			return face
		}
		telegramFontSet = imageFonts{
			title: newFace(44), badge: newFace(21), meta: newFace(20),
			ip: newFace(58), ipCompact: newFace(40), label: newFace(27),
			source: newFace(17), value: newFace(29), valueSmall: newFace(22),
			arrow: newFace(28), status: newFace(38),
		}
	})
	if telegramFontErr != nil {
		return imageFonts{}, telegramFontErr
	}
	return telegramFontSet, nil
}

func buildTelegramImageContent(event eventEnvelope) imageContent {
	content := imageContent{locale: event.Locale}
	content.title, _ = renderEvent(event.Locale, event)
	content.meta = telegramEventMeta(event)
	if event.Egress != nil {
		content.address = event.Egress.Name
		content.family = strings.ToUpper(event.Egress.Family)
	}

	if event.Type == EventProbeFieldChange {
		var data ProbeChangeData
		if json.Unmarshal(event.Data, &data) == nil {
			content.badge = localizedNotification(event.Locale,
				fmt.Sprintf("%d changes", len(data.Changes)), fmt.Sprintf("%d 项变化", len(data.Changes)))
			content.badgeTone = toneAmber
			visible := data.Changes[:min(len(data.Changes), maximumImageChanges)]
			content.changes = make([]imageChange, 0, len(visible))
			for _, change := range visible {
				name, ok := probefields.DisplayName(change.FieldID, event.Locale)
				if !ok {
					name = localizedNotification(event.Locale, "Unknown probe field", "未知探测字段")
				}
				before := truncateImageValue(probefields.DisplayValue(change.FieldID, change.Before, event.Locale), 256)
				after := truncateImageValue(probefields.DisplayValue(change.FieldID, change.After, event.Locale), 256)
				content.changes = append(content.changes, imageChange{
					label: name, source: probeFieldGroup(change.FieldID, event.Locale),
					before: before, beforeTone: probeValueTone(change.FieldID, change.Before, before),
					after: after, afterTone: probeValueTone(change.FieldID, change.After, after),
				})
			}
			content.remaining = len(data.Changes) - len(visible)
		}
		return content
	}

	content.status, content.statusTone, content.facts = genericEventContent(event)
	content.badge = genericEventBadge(event.Type, event.Locale)
	content.badgeTone = content.statusTone
	return content
}

func genericEventBadge(eventType, locale string) string {
	switch eventType {
	case EventAddressCheckFailure, EventProbeFailure, EventFormatMismatch:
		return localizedNotification(locale, "Alert", "异常")
	case EventAddressCheckRecover, EventProbeRecovery, EventFormatRecovery:
		return localizedNotification(locale, "Recovered", "已恢复")
	case EventAddressChange, EventFormatChanged:
		return localizedNotification(locale, "Changed", "发生变化")
	case EventAddressGap, EventProbeGap:
		return localizedNotification(locale, "Data gap", "数据缺口")
	case EventTest:
		return localizedNotification(locale, "Test", "测试")
	default:
		return localizedNotification(locale, "Event", "事件")
	}
}

func telegramEventMeta(event eventEnvelope) string {
	owner := localizedNotification(event.Locale, "IPChronicle Center", "IPChronicle 中心")
	if event.Node != nil {
		owner = event.Node.Name
	}
	observed, err := time.Parse(time.RFC3339, event.ObservedAt)
	if err != nil {
		return owner
	}
	return owner + "  ·  " + observed.UTC().Format("01-02 15:04 UTC")
}

func genericEventContent(event eventEnvelope) (string, imageTone, []imageFact) {
	switch event.Type {
	case EventAddressChange, EventAddressCheckFailure, EventAddressCheckRecover:
		return addressImageContent(event)
	case EventProbeFailure, EventProbeRecovery:
		var data ProbeOutcomeData
		_ = json.Unmarshal(event.Data, &data)
		if event.Type == EventProbeRecovery {
			return localizedNotification(event.Locale, "Complete probe succeeded", "完整探测成功"), toneGreen, nil
		}
		if data.FailureStage != nil {
			return probeFailureStage(*data.FailureStage, event.Locale), toneRed, nil
		}
		return localizedNotification(event.Locale, "Complete probe failed", "完整探测失败"), toneRed, nil
	case EventAddressGap, EventProbeGap:
		var data GapData
		_ = json.Unmarshal(event.Data, &data)
		facts := []imageFact{
			{label: localizedNotification(event.Locale, "Sequence range", "序号范围"), value: fmt.Sprintf("%d - %d", data.FirstSequence, data.LastSequence), tone: toneAmber},
		}
		status := localizedNotification(event.Locale,
			fmt.Sprintf("%d records were not retained", data.DroppedCount),
			fmt.Sprintf("%d 条记录未能保留", data.DroppedCount))
		return status, toneAmber, facts
	case EventFormatMismatch, EventFormatChanged, EventFormatRecovery:
		var data FormatData
		_ = json.Unmarshal(event.Data, &data)
		if event.Type == EventFormatRecovery {
			return localizedNotification(event.Locale, "Expected fields restored", "预期字段已恢复"), toneGreen, nil
		}
		tone := toneRed
		if event.Type == EventFormatChanged {
			tone = toneAmber
		}
		status := localizedNotification(event.Locale,
			fmt.Sprintf("%d format issues", data.IssueCount), fmt.Sprintf("%d 项格式问题", data.IssueCount))
		return status, tone, nil
	case EventTest:
		return localizedNotification(event.Locale, "Telegram accepted this message", "Telegram 已接收这条消息"), toneGreen, nil
	default:
		return localizedNotification(event.Locale, "Event recorded", "事件已记录"), toneBlue, nil
	}
}

func addressImageContent(event eventEnvelope) (string, imageTone, []imageFact) {
	var data AddressData
	_ = json.Unmarshal(event.Data, &data)
	status := localizedNotification(event.Locale, "Address changed", "地址发生变化")
	tone := toneAmber
	switch event.Type {
	case EventAddressCheckFailure:
		status, tone = localizedNotification(event.Locale, "Public address detection failed", "公网地址检测失败"), toneRed
	case EventAddressCheckRecover:
		status, tone = localizedNotification(event.Locale, "Public address detection recovered", "公网地址检测已恢复"), toneGreen
	case EventAddressChange:
		if data.Kind == "address-added" {
			status, tone = localizedNotification(event.Locale, "Entered the current address set", "已进入当前地址集合"), toneGreen
		} else if data.Kind == "address-removed" {
			status, tone = localizedNotification(event.Locale, "Left the current address set", "已离开当前地址集合"), toneRed
		}
	}
	facts := make([]imageFact, 0, 3)
	if data.FailureReason != nil {
		status = addressFailureReason(*data.FailureReason, event.Locale)
	}
	path := localizedNotification(event.Locale, "Direct from node", "节点直连")
	pathTone := toneBlue
	if data.ProxyPath {
		path = localizedNotification(event.Locale, "Network proxy", "网络代理")
		pathTone = toneAmber
	}
	facts = append(facts, imageFact{label: localizedNotification(event.Locale, "Discovery path", "发现路径"), value: path, tone: pathTone})
	if data.LikelyNAT {
		facts = append(facts, imageFact{label: localizedNotification(event.Locale, "Network mapping", "网络映射"), value: localizedNotification(event.Locale, "Likely NAT", "疑似 NAT"), tone: toneAmber})
	}
	if data.Temporary {
		facts = append(facts, imageFact{label: localizedNotification(event.Locale, "Source address", "源地址"), value: localizedNotification(event.Locale, "Temporary IPv6", "临时 IPv6"), tone: toneAmber})
	}
	return status, tone, facts
}

func addressFailureReason(value, locale string) string {
	values := map[string][2]string{
		"selector-unavailable":     {"Configured source is unavailable", "配置的源地址不可用"},
		"no-valid-response":        {"No valid discovery response", "未收到有效的发现结果"},
		"confirmation-unavailable": {"Independent confirmation unavailable", "独立确认服务不可用"},
		"conflicting-responses":    {"Discovery services returned different addresses", "发现服务返回了不同地址"},
	}
	if translated, ok := values[value]; ok {
		if locale == "zh-CN" {
			return translated[1]
		}
		return translated[0]
	}
	return localizedNotification(locale, "Address detection failed", "公网地址检测失败")
}

func probeFailureStage(value, locale string) string {
	values := map[string][2]string{
		"selector": {"Egress selection", "出口选择"},
		"adapter":  {"Probe adapter", "探测适配"},
		"process":  {"Probe execution", "探测执行"},
		"timeout":  {"Probe timeout", "探测超时"},
		"output":   {"Result processing", "结果处理"},
		"restart":  {"Agent restart", "Agent 重启"},
	}
	if translated, ok := values[value]; ok {
		if locale == "zh-CN" {
			return translated[1]
		}
		return translated[0]
	}
	return localizedNotification(locale, "Complete probe", "完整探测")
}

func probeFieldGroup(fieldID, locale string) string {
	group := strings.SplitN(fieldID, ".", 2)[0]
	groups := map[string][2]string{
		"Info":   {"Basic information", "基础信息"},
		"Type":   {"Database classification", "数据库分类"},
		"Score":  {"Risk score", "风险评分"},
		"Factor": {"Risk factor", "风险因子"},
		"Media":  {"Media availability", "媒体解锁"},
		"Mail":   {"Mail connectivity", "邮件连通"},
	}
	if translated, ok := groups[group]; ok {
		if locale == "zh-CN" {
			return translated[1]
		}
		return translated[0]
	}
	return localizedNotification(locale, "Probe result", "探测结果")
}

func probeValueTone(fieldID, raw, display string) imageTone {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "-" || trimmed == "null" || strings.EqualFold(trimmed, `"null"`) ||
		strings.EqualFold(display, "No data") || display == "无数据" {
		return toneNeutral
	}
	segments := strings.Split(fieldID, ".")
	if len(segments) >= 2 && segments[0] == "Score" {
		value, err := strconv.ParseFloat(strings.Trim(trimmed, `"`), 64)
		if err == nil {
			return riskScoreTone(segments[1], value)
		}
	}
	if len(segments) >= 2 && segments[0] == "Factor" {
		value, err := strconv.ParseBool(trimmed)
		if err == nil {
			if value {
				return toneRed
			}
			return toneGreen
		}
	}
	if len(segments) >= 2 && segments[0] == "Mail" {
		if len(segments) == 3 && segments[1] == "DNSBlacklist" {
			switch segments[2] {
			case "Clean":
				return toneGreen
			case "Marked":
				return toneAmber
			case "Blacklisted":
				return toneRed
			default:
				return toneBlue
			}
		}
		value, err := strconv.ParseBool(trimmed)
		if err == nil {
			if value {
				return toneGreen
			}
			return toneRed
		}
	}
	if len(segments) >= 2 && segments[0] == "Type" {
		return classificationTone(display)
	}
	if len(segments) >= 2 && segments[0] == "Media" {
		return mediaValueTone(display)
	}
	if fieldID == "Info.Type" {
		normalized := strings.ToLower(strings.TrimSpace(display))
		if normalized == "geo-consistent" || normalized == "原生ip" {
			return toneGreen
		}
		if normalized == "geo-discrepant" || normalized == "广播ip" {
			return toneRed
		}
	}
	return toneNeutral
}

func riskScoreTone(provider string, value float64) imageTone {
	switch provider {
	case "IP2LOCATION":
		if value < 33 {
			return toneGreen
		}
		if value < 66 {
			return toneAmber
		}
	case "SCAMALYTICS":
		if value < 20 {
			return toneGreen
		}
		if value < 60 {
			return toneAmber
		}
	case "ipapi":
		if value < .85 {
			return toneGreen
		}
		if value < 3 {
			return toneAmber
		}
	case "AbuseIPDB":
		if value < 25 {
			return toneGreen
		}
	case "IPQS":
		if value < 75 {
			return toneGreen
		}
		if value < 85 {
			return toneAmber
		}
	case "DBIP":
		if value == 0 {
			return toneGreen
		}
		if value == 50 {
			return toneAmber
		}
		if value != 100 {
			return toneNeutral
		}
	default:
		return toneNeutral
	}
	return toneRed
}

func classificationTone(value string) imageTone {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if stringIn(normalized, "hosting", "cdn", "web spider", "机房", "蜘蛛") {
		return toneRed
	}
	if stringIn(normalized, "isp", "line isp", "mobile isp", "家宽", "手机") {
		return toneGreen
	}
	if stringIn(normalized, "business", "education", "government", "banking", "organization", "military", "library", "reserved", "other", "商业", "教育", "政府", "银行", "组织", "军队", "图书馆", "保留", "其他") {
		return toneAmber
	}
	return toneNeutral
}

func mediaValueTone(value string) imageTone {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if stringIn(normalized, "yes", "unlocked", "解锁", "native", "原生") {
		return toneGreen
	}
	if stringIn(normalized, "block", "blocked", "屏蔽", "failed", "失败", "china", "中国", "noprem.", "禁会员") {
		return toneRed
	}
	if stringIn(normalized, "pending", "待支持", "nf.only", "仅自制", "webonly", "仅网页", "apponly", "仅app", "idc", "机房", "viadns", "dns") {
		return toneAmber
	}
	return toneNeutral
}

func stringIn(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func telegramImageHeight(content imageContent) int {
	headerBottom := 286
	if content.address == "" {
		headerBottom = 185
	}
	if len(content.changes) > 0 {
		height := headerBottom + 28 + len(content.changes)*100 + 32
		if content.remaining > 0 {
			height += 42
		}
		return height
	}
	height := headerBottom + 32 + 78 + len(content.facts)*64 + 40
	if height < 400 {
		return 400
	}
	return height
}

func drawTelegramImage(canvas *image.RGBA, fonts imageFonts, content imageContent) {
	innerWidth := telegramImageWidth - telegramImageMargin*2
	drawTextTop(canvas, fonts.title, content.title, telegramImageMargin, 46, 750, telegramPalette.text, true)
	if content.badge != "" {
		badgeWidth := min(max(measure(fonts.badge, content.badge)+48, 150), 280)
		drawBadge(canvas, telegramImageWidth-telegramImageMargin-badgeWidth, 43, badgeWidth, 60, 30, content.badgeTone)
		drawCenteredText(canvas, fonts.badge, content.badge, telegramImageWidth-telegramImageMargin-badgeWidth, 43, badgeWidth, 60, toneColor(content.badgeTone), true)
	}
	if content.meta != "" {
		drawTextTop(canvas, fonts.meta, content.meta, telegramImageMargin, 132, innerWidth, telegramPalette.muted, false)
	}
	headerBottom := 185
	if content.address != "" {
		ipFace := fonts.ip
		if measure(ipFace, content.address) > 760 {
			ipFace = fonts.ipCompact
		}
		drawTextTop(canvas, ipFace, content.address, telegramImageMargin, 188, 780, telegramPalette.text, true)
		if content.family != "" {
			drawBadge(canvas, 884, 205, 136, 60, 30, toneBlue)
			drawCenteredText(canvas, fonts.badge, content.family, 884, 205, 136, 60, telegramPalette.blue, true)
		}
		headerBottom = 286
	}
	drawHorizontalLine(canvas, telegramImageMargin, headerBottom, telegramImageWidth-telegramImageMargin, telegramPalette.border)

	if len(content.changes) > 0 {
		drawImageChanges(canvas, fonts, content, headerBottom+28)
		return
	}
	drawGenericEvent(canvas, fonts, content, headerBottom+32)
}

func drawImageChanges(canvas *image.RGBA, fonts imageFonts, content imageContent, top int) {
	for index, change := range content.changes {
		drawTextTop(canvas, fonts.label, change.label, 60, top+7, 258, telegramPalette.text, true)
		drawTextTop(canvas, fonts.source, change.source, 60, top+50, 258, telegramPalette.muted, false)
		drawValueBadge(canvas, fonts, change.before, change.beforeTone, 347, top+3, 260, 72)
		drawCenteredText(canvas, fonts.arrow, "→", 617, top+3, 54, 72, telegramPalette.muted, false)
		drawValueBadge(canvas, fonts, change.after, change.afterTone, 681, top+3, 339, 72)
		if index < len(content.changes)-1 {
			drawHorizontalLine(canvas, 60, top+90, 1020, telegramPalette.border)
		}
		top += 100
	}
	if content.remaining > 0 {
		message := localizedNotification(content.locale,
			fmt.Sprintf("%d more changes are available in the details", content.remaining),
			fmt.Sprintf("另有 %d 项变化，请打开详情查看", content.remaining))
		drawTextTop(canvas, fonts.meta, message, 60, top+4, 960, telegramPalette.muted, false)
	}
}

func drawGenericEvent(canvas *image.RGBA, fonts imageFonts, content imageContent, top int) {
	drawBadge(canvas, 60, top, 960, 78, 10, content.statusTone)
	drawCenteredText(canvas, fonts.status, content.status, 60, top, 960, 78, toneColor(content.statusTone), true)
	top += 104
	for index, fact := range content.facts {
		drawTextTop(canvas, fonts.meta, fact.label, 60, top+9, 260, telegramPalette.muted, false)
		drawTextTop(canvas, fonts.valueSmall, fact.value, 340, top+5, 680, toneColor(fact.tone), true)
		if index < len(content.facts)-1 {
			drawHorizontalLine(canvas, 60, top+55, 1020, telegramPalette.border)
		}
		top += 64
	}
}

func drawValueBadge(canvas *image.RGBA, fonts imageFonts, value string, tone imageTone, x, y, width, height int) {
	drawBadge(canvas, x, y, width, height, 12, tone)
	face := fonts.value
	if measure(face, value) > width-32 {
		face = fonts.valueSmall
	}
	drawCenteredText(canvas, face, value, x, y, width, height, toneColor(tone), true)
}

func drawBadge(canvas *image.RGBA, x, y, width, height, radius int, tone imageTone) {
	fill, edge := tonePalette(tone)
	drawRoundedRect(canvas, image.Rect(x, y, x+width, y+height), radius, edge)
	drawRoundedRect(canvas, image.Rect(x+2, y+2, x+width-2, y+height-2), max(radius-2, 0), fill)
}

func drawRoundedRect(destination *image.RGBA, rectangle image.Rectangle, radius int, fill color.Color) {
	if radius <= 0 {
		draw.Draw(destination, rectangle, image.NewUniform(fill), image.Point{}, draw.Src)
		return
	}
	radius = min(radius, min(rectangle.Dx(), rectangle.Dy())/2)
	draw.Draw(destination, image.Rect(rectangle.Min.X+radius, rectangle.Min.Y, rectangle.Max.X-radius, rectangle.Max.Y), image.NewUniform(fill), image.Point{}, draw.Src)
	draw.Draw(destination, image.Rect(rectangle.Min.X, rectangle.Min.Y+radius, rectangle.Max.X, rectangle.Max.Y-radius), image.NewUniform(fill), image.Point{}, draw.Src)
	radiusSquared := radius * radius
	centers := [][2]int{
		{rectangle.Min.X + radius, rectangle.Min.Y + radius},
		{rectangle.Max.X - radius - 1, rectangle.Min.Y + radius},
		{rectangle.Min.X + radius, rectangle.Max.Y - radius - 1},
		{rectangle.Max.X - radius - 1, rectangle.Max.Y - radius - 1},
	}
	for _, center := range centers {
		for offsetY := -radius; offsetY <= radius; offsetY++ {
			for offsetX := -radius; offsetX <= radius; offsetX++ {
				if offsetX*offsetX+offsetY*offsetY <= radiusSquared {
					destination.Set(center[0]+offsetX, center[1]+offsetY, fill)
				}
			}
		}
	}
}

func drawTextTop(destination draw.Image, face font.Face, value string, x, top, maximumWidth int, fill color.Color, bold bool) {
	value = fitText(face, strings.TrimSpace(value), maximumWidth)
	drawer := font.Drawer{Dst: destination, Src: image.NewUniform(fill), Face: face}
	drawer.Dot = fixed.P(x, top+face.Metrics().Ascent.Ceil())
	drawer.DrawString(value)
	if bold {
		drawer.Dot = fixed.P(x+1, top+face.Metrics().Ascent.Ceil())
		drawer.DrawString(value)
	}
}

func drawCenteredText(destination draw.Image, face font.Face, value string, x, y, width, height int, fill color.Color, bold bool) {
	value = fitText(face, strings.TrimSpace(value), width-24)
	textWidth := measure(face, value)
	metrics := face.Metrics()
	textHeight := (metrics.Ascent + metrics.Descent).Ceil()
	top := y + (height-textHeight)/2
	drawTextTop(destination, face, value, x+(width-textWidth)/2, top, width, fill, bold)
}

func drawHorizontalLine(destination *image.RGBA, left, y, right int, fill color.Color) {
	draw.Draw(destination, image.Rect(left, y, right, y+1), image.NewUniform(fill), image.Point{}, draw.Src)
}

func fitText(face font.Face, value string, maximumWidth int) string {
	if maximumWidth <= 0 || measure(face, value) <= maximumWidth {
		return value
	}
	const ellipsis = "…"
	limit := maximumWidth - measure(face, ellipsis)
	if limit <= 0 {
		return ellipsis
	}
	var fitted strings.Builder
	for _, character := range value {
		candidate := fitted.String() + string(character)
		if measure(face, candidate) > limit {
			break
		}
		fitted.WriteRune(character)
	}
	return fitted.String() + ellipsis
}

func measure(face font.Face, value string) int {
	return font.MeasureString(face, value).Ceil()
}

func truncateImageValue(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximum {
		return string(runes)
	}
	return string(runes[:maximum-1]) + "…"
}

func tonePalette(tone imageTone) (color.Color, color.Color) {
	switch tone {
	case toneGreen:
		return telegramPalette.greenSoft, telegramPalette.greenEdge
	case toneRed:
		return telegramPalette.redSoft, telegramPalette.redEdge
	case toneAmber:
		return telegramPalette.amberSoft, telegramPalette.amberEdge
	case toneBlue:
		return telegramPalette.blueSoft, telegramPalette.blueEdge
	default:
		return rgba(24, 24, 27), telegramPalette.border
	}
}

func toneColor(tone imageTone) color.Color {
	switch tone {
	case toneGreen:
		return telegramPalette.green
	case toneRed:
		return telegramPalette.red
	case toneAmber:
		return telegramPalette.amber
	case toneBlue:
		return telegramPalette.blue
	default:
		return telegramPalette.text
	}
}

func rgba(red, green, blue uint8) color.RGBA {
	return color.RGBA{R: red, G: green, B: blue, A: 255}
}
