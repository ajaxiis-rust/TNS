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
// роли, маршрут выбирает только провайдера и модель. Роли литературной
// обработки главы идут своим маршрутом (Э4, §6.3), остальные — маршрутом по
// умолчанию (sources).
type RoutedWorkshopSlot struct {
	routes RouteSlot
	byRole map[string]RouteSlot
}

// Slot (T-02-14): адаптер маршрута удовлетворяет контракту слота мастерской.
var _ Slot = (*RoutedWorkshopSlot)(nil)

// NewRoutedWorkshopSlot собирает слот мастерской по маршрутному слоту задачи;
// проводка ядра передаёт providerroute.Router.Slot(TaskSources).
func NewRoutedWorkshopSlot(routes RouteSlot) *RoutedWorkshopSlot {
	return &RoutedWorkshopSlot{routes: routes}
}

// RouteRoles отдаёт перечисленные роли маршруту другой задачи (Э4, §6.3):
// editor, editor_digest и style — один список маршрута editor, независимый от
// sources. Возвращает новый слот: слот, отданный в SetSlot, не меняется.
func (s *RoutedWorkshopSlot) RouteRoles(routes RouteSlot, roles ...string) *RoutedWorkshopSlot {
	out := &RoutedWorkshopSlot{
		routes: s.routes,
		byRole: make(map[string]RouteSlot, len(s.byRole)+len(roles)),
	}
	for role, r := range s.byRole {
		out.byRole[role] = r
	}
	for _, role := range roles {
		out.byRole[role] = routes
	}
	return out
}

// EditorRoles — роли литературной обработки готовой главы: правка, краткое
// содержание и дайджест, стилизация — один маршрут editor (§13.6).
func EditorRoles() []string {
	return []string{EditorRole, EditorDigestRole, StyleRole}
}

// routeFor выбирает маршрутный слот роли: роли литературной обработки главы —
// маршрут editor, прочие — маршрут по умолчанию.
func (s *RoutedWorkshopSlot) routeFor(role string) RouteSlot {
	if r, ok := s.byRole[role]; ok && r != nil {
		return r
	}
	return s.routes
}

// Generate выполняет один запрос маршрута роли: поток одной модели собирается в
// готовую строку. Отказ маршрута (нет ключей, все провайдеры отказали) уходит
// вызывающему: лестница GenerateWithRetry и ErrForgeDegraded не меняются.
func (s *RoutedWorkshopSlot) Generate(ctx context.Context, role, prompt string) (string, error) {
	if s == nil {
		return "", ErrRouteNotWired
	}
	routes := s.routeFor(role)
	if routes == nil {
		return "", ErrRouteNotWired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ch, err := routes.Stream(ctx, prompt)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	return b.String(), nil
}
