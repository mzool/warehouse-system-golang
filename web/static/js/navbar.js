

/////////////////////// navbar ///////////////////////
function initNavbar() {
  const navbar = document.querySelector("[data-navbar]");
  if (!navbar) return;

  // Mobile toggle
  navbar.addEventListener("click", (e) => {
    if (e.target.matches("[data-navbar-toggle]")) {
      document.body.classList.toggle("nav-open");
    }

    // Dropdown toggle
    if (e.target.matches("[data-dropdown-toggle]")) {
      const item = e.target.closest(".has-dropdown");
      const expanded = item.classList.toggle("is-open");
      e.target.setAttribute("aria-expanded", expanded);
    }
  });

  // Close dropdowns on outside click
  document.addEventListener("click", (e) => {
    if (!navbar.contains(e.target)) {
      navbar.querySelectorAll(".has-dropdown").forEach((el) => {
        el.classList.remove("is-open");
      });
    }
  });
}
