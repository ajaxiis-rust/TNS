// Тема до первой отрисовки (W1): без зависимостей, ставит data-theme на <html>.
(function () {
  var root = document.documentElement;
  var query = new URLSearchParams(window.location.search).get("theme");
  var theme = query === "dark" || query === "light" || query === "custom" ? query : "";
  if (!theme) {
    try {
      theme = window.localStorage.getItem("tns65.theme") || "dark";
    } catch (err) {
      theme = "dark";
    }
  }
  if (theme === "default") {
    theme = "dark";
  }
  root.dataset.theme = theme;
})();
