//go:build !e2e

package webapi

// e2eRoutes возвращает маршруты e2e-сборки. В релизной сборке их нет: фейковые
// часы и контрольный сдвиг времени недоступны игроку (T-09-02).
func e2eRoutes() []routeSpec { return nil }
