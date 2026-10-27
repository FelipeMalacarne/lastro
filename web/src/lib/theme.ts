// Light or dark follows the system. tokens.css switches every token on
// [data-theme="dark"], so the attribute on <html> is the whole mechanism.

export function followSystemTheme() {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  const apply = () => {
    document.documentElement.dataset.theme = media.matches ? "dark" : "light";
  };
  apply();
  media.addEventListener("change", apply);
}
