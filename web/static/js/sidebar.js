(function () {
    const sidebar = document.querySelector("[data-sidebar]");
    if (!sidebar) return;

    const openBtn = document.querySelector("[data-sidebar-open]");
    const closeBtn = sidebar.querySelector("[data-sidebar-close]");

    const open = () => sidebar.classList.add("is-open");
    const close = () => sidebar.classList.remove("is-open");

    openBtn?.addEventListener("click", open);
    closeBtn?.addEventListener("click", close);

    document.addEventListener("keydown", (e) => {
        if (e.key === "Escape") close();
    });
})();