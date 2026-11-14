package workshop

import (
	"context"
	"errors"
	"strings"
)

// RouteSlot — маршрутный слот задачи: провайдера на каждый запрос выбирает
// роутер (providerroute.RoutedProseSlot), ошибка до первого чанка уходит
// следующему провайдеру цепочки.
type RouteSlot interface {
	Stream(ctx context.Context, prompt string) (<-chan string, error)
}

// ErrRouteNotWired — маршрутный слот задачи не подключён: генерация идёт
// существующей деградацией.
var ErrRouteNotWired = errors.New("workshop: маршрут задачи не подключён")

// RoutedWorkshopSlot — слот мастерской поверх маршрута задачи (Э3,
// PROVIDER-ROUTING-PLAN §6.2): на каждый Generate провайдера выбирает роутер,
// кусок текста уходит одной модели одним запросом и целиком — дробления между
// провайдерами нет, ротация идёт между запросами. Промпт передаётся маршруту
// без изменений, роль остаётся у вызывающего: промпты и температура живут у
// роли, маршрут выбирает только провайдера и модель.
type RoutedWorkshopSlot struct {
	routes RouteSlot
}

// Slot (T-02-14): адаптер маршрута удовлетворяет контракту слота мастерской.
var _ Slot = (*RoutedWorkshopSlot)(nil)

// NewRoutedWorkshopSlot собирает слот мастерской по маршрутному слоту задачи;
// проводка ядра передаёт providerroute.Router.Slot(TaskSources).
func NewRoutedWorkshopSlot(routes RouteSlot) *RoutedWorkshopSlot {
	return &RoutedWorkshopSlot{routes: routes}
}

// Generate выполняет один запрос маршрута: поток одной модели собирается в
// готовую строку. Отказ маршрута (нет ключей, все провайдеры отказали) уходит
// вызывающему: лестница GenerateWithRetry и ErrForgeDegraded не меняются.
func (s *RoutedWorkshopSlot) Generate(ctx context.Context, _, prompt string) (string, error) {
	if s == nil || s.routes == nil {
		return "", ErrRouteNotWired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ch, err := s.routes.Stream(ctx, prompt)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	return b.String(), nil
}
