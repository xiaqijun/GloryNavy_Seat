// Run before styles/React so the saved theme also applies to the first paint.
(() => {
  let preference = "system";
  try { preference = localStorage.getItem("glorynavy.theme") || "system"; } catch {}
  const theme = preference === "dark" || preference !== "light" &&
    matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
})();
