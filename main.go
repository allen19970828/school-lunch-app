package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed web/*
var webFS embed.FS

// --- GORM 資料庫模型 (完整移植舊版 Student 與 Staff 屬性) ---

// Student 學生模型 (與舊版 AI-lunch-liff 100% 契合)
type Student struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ClassroomName string    `json:"classroom_name"` // 如: 三年二班
	SeatNumber    int       `json:"seat_number"`    // 座號
	StudentCode   string    `json:"student_code"`   // 學號
	Name          string    `json:"name"`           // 姓名
	Identity      string    `json:"identity"`       // general(一般) / low_income(低收) / mid_low_income(中低收) / disabled(身障) / teacher_certified(導師認定)
	DietaryType   string    `json:"dietary_type"`   // omnivore(葷食) / vegan(全素) / ovo_lacto(蛋奶素)
	Status        string    `json:"status"`         // active(在學) / transferred(轉出) / graduated(畢業)
	IsMealUser    bool      `json:"is_meal_user"`   // 是否搭餐
	CreatedAt     time.Time `json:"created_at"`
}

// Staff 教職員模型
type Staff struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`          // 行政主任 / 班導師 / 專任教師
	MealMode     string    `json:"meal_mode"`     // monthly / daily
	IsSubsidized bool      `json:"is_subsidized"` // 是否享有導師/導護補助
	SubReason    string    `json:"sub_reason"`    // 導師補助 / 導護補助
	CreatedAt    time.Time `json:"created_at"`
}

// MealCancellation 請假停餐單據模型
type MealCancellation struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TargetID     string    `json:"target_id"`
	TargetType   string    `json:"target_type"` // student / staff
	TargetName   string    `json:"target_name"`
	TargetDetail string    `json:"target_detail"`
	CancelDate   string    `json:"cancel_date"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"` // approved / pending / rejected
	ReviewNote   string    `json:"review_note"`
	RefundAmount float64   `json:"refund_amount"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuditLog 異動稽核日誌模型
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Operator  string    `json:"operator"` // 操作者
	Action    string    `json:"action"`   // 動作
	Details   string    `json:"details"`  // 詳細內容
	CreatedAt time.Time `json:"created_at"`
}

// MealComplaint 膳食滿意度模型
type MealComplaint struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Reporter  string    `json:"reporter"`
	Rating    int       `json:"rating"`
	DishName  string    `json:"dish_name"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	log.Println("🚀 正在啟動 school-lunch-app 完整後台管理系統 (含學生與教職員 CRUD)...")

	db, err := gorm.Open(sqlite.Open("school_lunch_demo.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 無法連線至 SQLite 資料庫: %v", err)
	}

	db.AutoMigrate(&Student{}, &Staff{}, &MealCancellation{}, &AuditLog{}, &MealComplaint{})
	log.Println("✅ 成功自動建表 (Student, Staff, MealCancellation, AuditLog, MealComplaint) 至 SQLite！")

	// 預塞學生與教職員初始測試資料 (符合舊專案結構)
	var studentCount int64
	db.Model(&Student{}).Count(&studentCount)
	if studentCount == 0 {
		db.Create(&Student{ClassroomName: "三年二班", SeatNumber: 1, StudentCode: "113001", Name: "林小明", Identity: "general", DietaryType: "omnivore", Status: "active", IsMealUser: true})
		db.Create(&Student{ClassroomName: "三年二班", SeatNumber: 2, StudentCode: "113002", Name: "陳美玲", Identity: "low_income", DietaryType: "vegan", Status: "active", IsMealUser: true})
		db.Create(&Student{ClassroomName: "五年一班", SeatNumber: 12, StudentCode: "111012", Name: "張家豪", Identity: "mid_low_income", DietaryType: "omnivore", Status: "active", IsMealUser: true})
		db.Create(&Student{ClassroomName: "五年一班", SeatNumber: 15, StudentCode: "111015", Name: "黃怡婷", Identity: "teacher_certified", DietaryType: "ovo_lacto", Status: "active", IsMealUser: true})
	}

	var staffCount int64
	db.Model(&Staff{}).Count(&staffCount)
	if staffCount == 0 {
		db.Create(&Staff{Name: "張主任 (總務處)", Role: "行政主任", MealMode: "monthly", IsSubsidized: false})
		db.Create(&Staff{Name: "陳老師 (三年二班)", Role: "班導師", MealMode: "monthly", IsSubsidized: true, SubReason: "導師補助"})
		db.Create(&Staff{Name: "林老師 (五年一班)", Role: "輪值導護", MealMode: "monthly", IsSubsidized: true, SubReason: "導護補助"})
		db.Create(&Staff{Name: "李組長 (午餐執秘)", Role: "午餐執秘", MealMode: "monthly", IsSubsidized: false})
		db.Create(&Staff{Name: "王組長 (訓導組)", Role: "專任教師", MealMode: "daily", IsSubsidized: false})
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

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

	subFS, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("無法載入靜態網頁: %v", err)
	}
	r.StaticFS("/ui", http.FS(subFS))

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/ui/")
	})

	api := r.Group("/api/v1")
	{
		// --- 1. 學生 Resource CRUD API ---
		api.GET("/students", func(c *gin.Context) {
			var students []Student
			db.Order("classroom_name asc, seat_number asc").Find(&students)
			c.JSON(http.StatusOK, gin.H{"data": students})
		})

		api.POST("/students", func(c *gin.Context) {
			var item Student
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.Status = "active"
			item.IsMealUser = true
			item.CreatedAt = time.Now()
			db.Create(&item)

			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "新增學生",
				Details:   fmt.Sprintf("新增 %s %d號 %s (身分: %s, 飲食: %s)", item.ClassroomName, item.SeatNumber, item.Name, item.Identity, item.DietaryType),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.PUT("/students/:id", func(c *gin.Context) {
			id := c.Param("id")
			var item Student
			if err := db.First(&item, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "找不到該學生"})
				return
			}
			var input Student
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.ClassroomName = input.ClassroomName
			item.SeatNumber = input.SeatNumber
			item.StudentCode = input.StudentCode
			item.Name = input.Name
			item.Identity = input.Identity
			item.DietaryType = input.DietaryType
			item.IsMealUser = input.IsMealUser
			db.Save(&item)

			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "修改學生",
				Details:   fmt.Sprintf("更新學生 ID#%s %s 資料 (身分: %s, 飲食: %s)", id, item.Name, item.Identity, item.DietaryType),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.DELETE("/students/:id", func(c *gin.Context) {
			id := c.Param("id")
			db.Delete(&Student{}, id)
			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "刪除學生",
				Details:   fmt.Sprintf("刪除學生 ID#%s", id),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		// --- 2. 教職員 Resource CRUD API ---
		api.GET("/staff", func(c *gin.Context) {
			var staffList []Staff
			db.Order("id asc").Find(&staffList)
			c.JSON(http.StatusOK, gin.H{"data": staffList})
		})

		api.POST("/staff", func(c *gin.Context) {
			var item Staff
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.CreatedAt = time.Now()
			db.Create(&item)

			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "新增教職員",
				Details:   fmt.Sprintf("新增教職員 %s (身分: %s, 模式: %s)", item.Name, item.Role, item.MealMode),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.PUT("/staff/:id", func(c *gin.Context) {
			id := c.Param("id")
			var item Staff
			if err := db.First(&item, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "找不到該教職員"})
				return
			}
			var input Staff
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.Name = input.Name
			item.Role = input.Role
			item.MealMode = input.MealMode
			item.IsSubsidized = input.IsSubsidized
			item.SubReason = input.SubReason
			db.Save(&item)

			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "修改教職員",
				Details:   fmt.Sprintf("更新 ID#%s %s 搭餐設定", id, item.Name),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": item})
		})

		api.DELETE("/staff/:id", func(c *gin.Context) {
			id := c.Param("id")
			db.Delete(&Staff{}, id)
			db.Create(&AuditLog{
				Operator:  "管理員",
				Action:    "刪除教職員",
				Details:   fmt.Sprintf("刪除教職員 ID#%s", id),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

		// --- 3. 請假單據 工作流 API ---
		api.GET("/cancellation/list", func(c *gin.Context) {
			var list []MealCancellation
			db.Order("id desc").Find(&list)
			c.JSON(http.StatusOK, gin.H{"data": list})
		})

		api.POST("/cancellation", func(c *gin.Context) {
			var req MealCancellation
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.Status = "approved"
			req.RefundAmount = 60.0
			req.CreatedAt = time.Now()
			db.Create(&req)

			db.Create(&AuditLog{
				Operator:  req.TargetName,
				Action:    "提交停餐單據",
				Details:   fmt.Sprintf("%s 申請 %s 停餐 (原因: %s)", req.TargetName, req.CancelDate, req.Reason),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "approved", "message": "單據已寫入 SQLite 資料庫！", "data": req})
		})

		api.PUT("/cancellation/:id/status", func(c *gin.Context) {
			id := c.Param("id")
			var req MealCancellation
			if err := db.First(&req, id).Error; err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "找不到單據"})
				return
			}
			var input struct {
				Status     string `json:"status"`
				ReviewNote string `json:"review_note"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.Status = input.Status
			req.ReviewNote = input.ReviewNote
			db.Save(&req)

			db.Create(&AuditLog{
				Operator:  "午餐執秘 (管理員)",
				Action:    "人工審核單據",
				Details:   fmt.Sprintf("單據 ID#%s 變更為 %s (備註: %s)", id, input.Status, input.ReviewNote),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "data": req})
		})

		// --- 4. 異動稽核日誌 API ---
		api.GET("/audit-logs", func(c *gin.Context) {
			var logs []AuditLog
			db.Order("id desc").Limit(20).Find(&logs)
			c.JSON(http.StatusOK, gin.H{"data": logs})
		})

		// 膳食滿意度 API
		api.POST("/complaint", func(c *gin.Context) {
			var item MealComplaint
			if err := c.ShouldBindJSON(&item); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			item.CreatedAt = time.Now()
			db.Create(&item)

			db.Create(&AuditLog{
				Operator:  item.Reporter,
				Action:    "提交膳食評價",
				Details:   fmt.Sprintf("%s 對菜色 [%s] 給予 %d 顆星評價", item.Reporter, item.DishName, item.Rating),
				CreatedAt: time.Now(),
			})
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "已寫入 SQLite 資料庫！"})
		})

		api.GET("/complaint/list", func(c *gin.Context) {
			var complaints []MealComplaint
			db.Order("id desc").Find(&complaints)
			c.JSON(http.StatusOK, gin.H{"list": complaints})
		})

		// 採購與結算 API
		api.POST("/procurement/draft", func(c *gin.Context) {
			draft := `【公務簽辦稿樣 - 免辦公開閱覽說明】
案由：臺中市神岡區豐洲國民小學辦理「115學年度學生午餐採購案」採購案，擬依《政府採購招標文件公開閱覽制度實施要點》第二點第3款規定免辦公開閱覽，簽請核示。

說明：
一、本案採購金額為新臺幣 12,000,000 元整。
二、查本案採購內容屬經常性、重複性辦理之學生午餐採購，其規格範本業經教育局審定，且前次招標未有重大規格爭議。
三、綜上，本案符合前開要點「重複性採購」得免辦公開閱覽之規定，擬免予公開閱覽，以提升採購效率。`
			c.JSON(http.StatusOK, gin.H{"official_document_draft": draft})
		})

		api.POST("/settlement/export-excel", func(c *gin.Context) {
			f := excelize.NewFile()
			sheet := "收支結算表(114_04版)"
			f.SetSheetName("Sheet1", sheet)
			f.SetCellValue(sheet, "A1", "臺中市神岡區豐洲國民小學\n經費收支結算表")
			f.SetCellValue(sheet, "A2", "計畫名稱：114學年度第1學期免費營養午餐補助")
			f.SetCellValue(sheet, "A6", "業務費(經常門)")
			f.SetCellValue(sheet, "B6", 296010)
			f.SetCellValue(sheet, "C6", 296010)
			f.SetCellValue(sheet, "E6", 284624)
			buffer, _ := f.WriteToBuffer()
			c.Header("Content-Disposition", "attachment; filename=\"豐洲國小_經費收支結算表.xlsx\"")
			c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
		})
	}

	fmt.Println("\n========================================================")
	fmt.Println("🎉 學校午餐 Full Admin 管理系統 (學生 + 教職員全 CRUD) 已成功啟動！")
	fmt.Println("🌐 請開啟瀏覽器訪問: http://localhost:8080/")
	fmt.Println("========================================================\n")

	if err := r.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("❌ 伺服器啟動失敗: %v", err)
	}
}
