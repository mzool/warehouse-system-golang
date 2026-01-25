function initTables() {
  document.querySelectorAll("[data-table]").forEach((table) => {
    const input = table.querySelector("[data-table-search]");
    const rows = table.querySelectorAll("tbody tr");

    if (!input) return;

    input.addEventListener("input", () => {
      const query = input.value.toLowerCase();

      rows.forEach((row) => {
        const text = row.innerText.toLowerCase();
        row.classList.toggle("is-hidden", !text.includes(query));
      });
    });
  });
}
