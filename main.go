package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

//go:embed web/*
var webFS embed.FS

// OfficialSettlementReport 經費收支結算表導出參數
type OfficialSettlementReport struct {
	SchoolName           string  `json:"school_name"`
	ProjectName          string  `json:"project_name"`
	BusinessPlanAmount   float64 `json:"business_plan_amount"`
	BusinessGrantAmount  float64 `json:"business_grant_amount"`
	BusinessSpentAmount  float64 `json:"business_spent_amount"`
	Note                 string  `json:"note"`
}

func main() {
	log.Println("🚀 正在啟動 school-lunch-app 學校午餐智慧管理 Demo 服務 (Go 一體化單檔)...")

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 1. CORS 跨域設定
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 2. 靜態網頁託管 (Go embed 一體化部署)
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("無法載入靜態網頁: %v", err)
	}
	r.StaticFS("/ui", http.FS(subFS))

	// 根目錄重新導向至 UI 介面
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	// 3. API 端點路由
	api := r.Group("/api/v1")
	{
		// 停餐請假申請 (含倒推 3 工作天 12:00 PM 截點檢核)
		api.POST("/cancellation", func(c *gin.Context) {
			var req struct {
				TargetName string `json:"target_name"`
				TargetType string `json:"target_type"`
				CancelDate string `json:"cancel_date"`
				Reason     string `json:"reason"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "無效的請求參數"})
				return
			}

			// 計算截點邏輯
			parseDate, err := time.Parse("2006-01-02", req.CancelDate)
			if err != nil {
				parseDate = time.Now().AddDate(0, 0, 4)
			}
			// 模擬倒推 3 工作天 12:00 PM
			deadline := parseDate.AddDate(0, 0, -3)
			deadline = time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 12, 0, 0, 0, time.Local)

			isApproved := true
			_ = isApproved // 用於 demo 標示通過狀態
			c.JSON(http.StatusOK, gin.H{
				"status":        "approved",
				"target_name":   req.TargetName,
				"cancel_date":   req.CancelDate,
				"deadline":      deadline.Format("2006-01-02 15:04:00"),
				"refund_amount": 60.0,
				"message":       fmt.Sprintf("請假成功！前 3 工作天 12:00 PM 截點為 %s。已完成備餐人數扣銷與退款登錄。", deadline.Format("2006-01-02 15:04:00")),
			})
		})

		// 115 補助試算 API
		api.POST("/subsidy/calc", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"education_level":      "elementary",
				"base_subsidy":         60.0,
				"location_bonus":       4.0,
				"organic_bonus":        4.0,
				"total_meal_subsidy":   68.0,
				"weekly_total_subsidy": 340.0,
				"policy_note":          "符合 115 免費午餐 60 元補助政策 + 一般學校 4 元加碼 + 有機米/蔬菜 4 元獎勵",
			})
		})

		// 採購簽辦稿樣 API
		api.POST("/procurement/draft", func(c *gin.Context) {
			var req struct {
				CaseNumber string  `json:"case_number"`
				CaseName   string  `json:"case_name"`
				Budget     float64 `json:"budget_amount"`
				School     string  `json:"school_name"`
				Vendor     string  `json:"vendor_name"`
			}
			_ = c.ShouldBindJSON(&req)
			if req.School == "" {
				req.School = "臺中市神岡區豐洲國民小學"
			}
			draft := fmt.Sprintf(`【公務簽辦稿樣 - 免辦公開閱覽說明】
案由：%s 辦理「%s」採購案，擬依《政府採購招標文件公開閱覽制度實施要點》第二點第3款規定免辦公開閱覽，簽請核示。

說明：
一、本案採購金額為新臺幣 %.0f 元整。
二、查本案採購內容屬經常性、重複性辦理之學生午餐採購，其規格範本業經教育局審定，且前次招標未有重大規格爭議。
三、綜上，本案符合前開要點「重複性採購」得免辦公開閱覽之規定，擬免予公開閱覽，以提升採購效率。`, req.School, req.CaseName, req.Budget)

			c.JSON(http.StatusOK, gin.H{
				"school_name":              req.School,
				"case_name":                req.CaseName,
				"official_document_draft": draft,
			})
		})

		// 114_04 版經費收支結算表 Excel 一鍵導出 API
		api.POST("/settlement/export-excel", func(c *gin.Context) {
			var input OfficialSettlementReport
			if err := c.ShouldBindJSON(&input); err != nil {
				input.SchoolName = "臺中市神岡區豐洲國民小學"
				input.ProjectName = "114學年度第1學期免費營養午餐補助"
				input.BusinessPlanAmount = 296010
				input.BusinessGrantAmount = 296010
				input.BusinessSpentAmount = 284624
				input.Note = "5元加碼剩餘款:9610\n健康飲食材料費剩餘款:1584"
			}

			// 動態生成格式精美的 Excel
			f := excelize.NewFile()
			sheet := "收支結算表(114_04版)"
			f.SetSheetName("Sheet1", sheet)

			f.SetCellValue(sheet, "A1", fmt.Sprintf("%s\n經費收支結算表", input.SchoolName))
			f.SetCellValue(sheet, "A2", fmt.Sprintf("計畫名稱：%s", input.ProjectName))
			f.SetCellValue(sheet, "D2", "單位：新臺幣元")

			headers := []string{"核定項目", "本局核定計畫金額(A)", "本局核定補助金額(B)", "本局核定補助比率(C=B/A)", "實支總額(D)", "計畫結餘款(E=A-D)", "應繳回本局結餘款(F)", "備註"}
			for idx, h := range headers {
				cell, _ := excelize.CoordinatesToCellName(idx+1, 4)
				f.SetCellValue(sheet, cell, h)
			}

			// 人事費
			f.SetCellValue(sheet, "A5", "人事費(經常門)")
			// 業務費
			f.SetCellValue(sheet, "A6", "業務費(經常門)")
			f.SetCellValue(sheet, "B6", input.BusinessPlanAmount)
			f.SetCellValue(sheet, "C6", input.BusinessGrantAmount)
			f.SetCellValue(sheet, "D6", 1.0)
			f.SetCellValue(sheet, "E6", input.BusinessSpentAmount)
			surplus := input.BusinessPlanAmount - input.BusinessSpentAmount
			f.SetCellValue(sheet, "F6", surplus)
			f.SetCellValue(sheet, "G6", math.Ceil(surplus))
			f.SetCellValue(sheet, "H6", input.Note)

			// 合計
			f.SetCellValue(sheet, "A8", "合計")
			f.SetCellValue(sheet, "B8", input.BusinessPlanAmount)
			f.SetCellValue(sheet, "C8", input.BusinessGrantAmount)
			f.SetCellValue(sheet, "E8", input.BusinessSpentAmount)
			f.SetCellValue(sheet, "F8", surplus)
			f.SetCellValue(sheet, "G8", math.Ceil(surplus))

			// 分攤表
			f.SetCellValue(sheet, "A10", "支出機關分攤表：")
			f.SetCellValue(sheet, "A11", "分攤機關名稱")
			f.SetCellValue(sheet, "B11", "分攤金額(元)")
			f.SetCellValue(sheet, "A12", "1. 臺中市政府教育局")
			f.SetCellValue(sheet, "B12", input.BusinessSpentAmount)

			buffer, _ := f.WriteToBuffer()

			fileName := fmt.Sprintf("%s_經費收支結算表.xlsx", input.SchoolName)
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
			c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
		})
	}

	fmt.Println("\n========================================================")
	fmt.Println("🎉 學校午餐 Demo 系統已成功啟動！")
	fmt.Println("🌐 請開啟瀏覽器訪問: http://localhost:8080/")
	fmt.Println("========================================================\n")

	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("❌ 伺服器啟動失敗: %v", err)
	}
}
