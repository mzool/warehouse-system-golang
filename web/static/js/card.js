function initCards() {
  document.addEventListener("click", (e) => {
    const card = e.target.closest("[data-card-link]");
    if (!card) return;

    const link = card.getAttribute("data-card-link");
    if (link) {
      window.location.href = link;
    }
  });
}
