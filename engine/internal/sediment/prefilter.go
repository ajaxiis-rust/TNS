package sediment

import "errors"

// ErrPrefiltered — префильтр отсеял сцену (W-04.7): конвейер останавливается до
// очистки, запись в литературную базу не выполняется.
var ErrPrefiltered = errors.New("sediment: сцена отсеяна префильтром")

// Prefilter — префильтр конвейера оседания (T-04-07, шаг 1). Принимает сцену и
// решает, пропустить её дальше (Keep=true) или отсеять до очистки. Реализация
// Паганеля — задача T-06-14; здесь интерфейс и точка вызова.
type Prefilter interface {
	Keep(scene Scene) bool
}

// passPrefilter — дефолтный префильтр: пропускает любую сцену, конвейер без
// Паганеля работает (T-04-07, шаг 2).
type passPrefilter struct{}

func (passPrefilter) Keep(Scene) bool { return true }
