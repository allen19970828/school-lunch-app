# 🍱 臺中市神岡區豐洲國小 - 學校午餐智慧管理系統 Demo (`school-lunch-app`)

> 🚀 **展示專案**：基於 [school-lunch-v2](https://github.com/allen19970828/school-lunch-v2) 高效能 Go 核心引擎打造之全功能智慧管理系統 Demo。  
> ⚡ **技術棧**：Go 1.22 (embed 一體化單檔) / Alpine.js 3.x / Tailwind CSS v4 / ECharts 5.x / Mermaid.js 10.x / Gin RESTful API

---

## 🌟 展示重點 (Key Highlights)

1. 📱 **導師/教職員極速請假 Demo (模擬 LINE LIFF)**：
   - 輸入欲停餐日期，動態觸發 Go Core 進行**前 3 工作日 12:00 PM 截點倒推檢核**。
   - 畫面即時透過 **Mermaid.js** 動態畫出請假審核時序流程圖！
2. 🎯 **主副食 70% 支出比例動態監控 (ECharts 儀表圖)**：
   - 即時監控主副食支出佔比是否低於 70% 警示線。
3. 📊 **115 免費午餐 60 元補助月度支用趨勢圖 (ECharts 長條圖)**：
   - 直觀展示核定計畫金額 (A) 與實支總額 (D) 支用狀況。
4. 📄 **一鍵導出臺中市 114_04 版經費收支結算表 `.xlsx`**：
   - 100% 讀取官方範本格式，帶入公式直接下載。
5. 📝 **2026/07/01 採購要點免公開閱覽簽辦稿樣生成**。

---

## 🚀 3 秒鐘啟動體驗

```bash
git clone https://github.com/allen19970828/school-lunch-app.git
cd school-lunch-app
go run main.go
```

開啟瀏覽器訪問：`http://localhost:8080/` 即可進行完整互動 Demo！
