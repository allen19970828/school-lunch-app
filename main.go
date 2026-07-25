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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed web/*
var webFS embed.FS

// --- GORM 資料庫模型 ---

// MealComplaint 膳食滿意度與意見反應模型
type MealComplaint struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Reporter  string    `json:"reporter"` // 填寫人 (如：陳老師 三年二班)
	Rating    int       `json:"rating"`   // 1-5 顆星
	DishName  string    `json:"dish_name"`// 菜色名稱 (如：高麗菜)
	Content   string    `json:"content"`  // 意見內容 (如：高麗菜太鹹)
	CreatedAt time.Time `json:"created_at"`
}

// Student 學生名冊模型
type Student struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ClassCode string `json:"class_code"` // 班級 (如：302)
	SeatNo    int    `json:"seat_no"`    // 座號
	Name      string `json:"name"`       // 姓名
}

// Staff 教職員名冊模型
type Staff struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Name     string `json:"name"`      // 姓名
	Role     string `json:"role"`      // 職務身分 (行政/導師/專任)
	MealMode string `json:"meal_mode"` // monthly / daily
}

// OfficialSettlementReport 經費收支結算表導出參數
type OfficialSettlementReport struct {
	SchoolName          string  `json:"school_name"`
	ProjectName         string  `json:"project_name"`
	BusinessPlanAmount  float64 `json:"business_plan_amount"`
	BusinessGrantAmount float64 `json:"business_grant_amount"`
	BusinessSpentAmount float64 `json:"business_spent_amount"`
	Note                string  `json:"note"`
}

func main() {
	log.Println("🚀 正在啟動 school-lunch-app (連線 SQLite 資料庫持久層)...")

	// 1. 連線 GORM SQLite 本地資料庫 (檔案：school_lunch_demo.db)
	db, err := gorm.Open(sqlite.Open("school_lunch_demo.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 無法連線至 SQLite 資料庫: %v", err)
	}

	// 自動建立 Schema (AutoMigrate)
	db.AutoMigrate(&MealComplaint{}, &Student{}, &Staff{})
	log.Println("✅ 成功自動建表 (MealComplaint, Student, Staff) 至 SQLite！")

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 2. CORS 跨域設定
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

	// 3. 靜態網頁託管 (Go embed 一體化部署)
	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("無法載入靜態網頁: %v", err)
	}
	r.StaticFS("/ui", http.FS(subFS))

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	// 4. API 端點路由
	api := r.Group("/api/v1")
	{
		// 提交停餐請假
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

			parseDate, err := time.Parse("2006-01-02", req.CancelDate)
			if err != nil {
				parseDate = time.Now().AddDate(0, 0, 4)
			}
			deadline := parseDate.AddDate(0, 0, -3)
			deadline = time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 12, 0, 0, 0, time.Local)

			c.JSON(http.StatusOK, gin.H{
				"status":        "approved",
				"target_name":   req.TargetName,
				"cancel_date":   req.CancelDate,
				"deadline":      deadline.Format("2006-01-02 15:04:00"),
				"refund_amount": 60.0,
				"message":       fmt.Sprintf("請假成功！前 3 工作天 12:00 PM 截點為 %s。已完成備餐人數扣銷與退款登錄。", deadline.Format("2006-01-02 15:04:00")),
			})
		})

		// ⭐ 膳食滿意度與意見反應 API (新增寫入 SQLite)
		api.POST("/complaint", func(c *gin.Context) {
			var item MealComplaint
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "無效的評價參數"})
				return
			}
			item.CreatedAt = time.Now()
			db.Create(&item)

			c.JSON(http.StatusOK, gin.H{
				"status":  "success",
				"message": "感謝您的意見反饋！已將評價與意見存入資料庫，並即時通知團膳業者與廚房。",
				"data":    item,
			})
		})

		// 查詢膳食意見彙整列表 API (從 SQLite 讀取)
		api.GET("/complaint/list", func(c *gin.Context) {
			var complaints []MealComplaint
			db.Order("id desc").Find(&complaints)
			c.JSON(http.StatusOK, gin.H{
				"total": len(complaints),
				"list":  complaints,
			})
		})

		// 📥 師生名冊 Excel 批次匯入 API (解析 `.xlsx` 並寫入 SQLite)
		api.POST("/import/roster", func(c *gin.Context) {
			file, err := c.FormFile("file")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "請上傳 Excel 檔案 (.xlsx)"})
				return
			}

			src, err := file.Open()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "無法開啟上傳檔案"})
				return
			}
			defer src.Close()

			excel, err := excelize.OpenReader(src)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "無效的 Excel 試算表格式"})
				return
			}
			defer excel.Close()

			rows, _ := excel.GetRows("Sheet1")
			importedCount := 0
			for idx, row := range rows {
				if idx == 0 || len(row) < 3 {
					continue // 跳過表頭
				}
				// 假設結構: 班級, 座號/職務, 姓名
				classCode := row[0]
				seatOrRole := row[1]
				name := row[2]

				if classCode == "教職員" {
					db.Create(&Staff{Name: name, Role: seatOrRole, MealMode: "monthly"})
				} else {
					db.Create(&Student{ClassCode: classCode, SeatNo: 1, Name: name})
				}
				importedCount++
			}

			c.JSON(http.StatusOK, gin.H{
				"status":         "success",
				"imported_count": importedCount,
				"message":        fmt.Sprintf("成功匯入 %d 筆全校師生名冊至 SQLite 資料庫！", importedCount),
			})
		})

		// 查詢全校師生名冊統計 API
		api.GET("/roster/summary", func(c *gin.Context) {
			var studentCount, staffCount int64
			db.Model(&Student{}).Count(&studentCount)
			db.Model(&Staff{}).Count(&staffCount)

			c.JSON(http.StatusOK, gin.H{
				"student_count": studentCount,
				"staff_count":   staffCount,
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
				"school_name":             req.School,
				"case_name":               req.CaseName,
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

			f.SetCellValue(sheet, "A5", "人事費(經常門)")
			f.SetCellValue(sheet, "A6", "業務費(經常門)")
			f.SetCellValue(sheet, "B6", input.BusinessPlanAmount)
			f.SetCellValue(sheet, "C6", input.BusinessGrantAmount)
			f.SetCellValue(sheet, "D6", 1.0)
			f.SetCellValue(sheet, "E6", input.BusinessSpentAmount)
			surplus := input.BusinessPlanAmount - input.BusinessSpentAmount
			f.SetCellValue(sheet, "F6", surplus)
			f.SetCellValue(sheet, "G6", math.Ceil(surplus))
			f.SetCellValue(sheet, "H6", input.Note)

			f.SetCellValue(sheet, "A8", "合計")
			f.SetCellValue(sheet, "B8", input.BusinessPlanAmount)
			f.SetCellValue(sheet, "C8", input.BusinessGrantAmount)
			f.SetCellValue(sheet, "E8", input.BusinessSpentAmount)
			f.SetCellValue(sheet, "F8", surplus)
			f.SetCellValue(sheet, "G8", math.Ceil(surplus))

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
