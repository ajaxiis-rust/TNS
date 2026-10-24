package db

// WorldsRoot — корень каталога миров (world_db/): миры живут в worlds/<slug>/,
// замороженная base.db — в base/ (T-01-16). Переменная параметризует корень,
// чтобы тесты рождения работали в t.TempDir().
var WorldsRoot = "world_db"
