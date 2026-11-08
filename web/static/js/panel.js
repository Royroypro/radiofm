/* =====================================================================
   RadioClub25 — utilidades de los paneles (admin y superadmin).
     - menú móvil
     - confirmación antes de borrar (data-confirm en <form>, data-confirm-click en botones)
     - botones «Copiar» (data-copy)
     - selector de color sincronizado con su campo de texto (data-color-for / data-color-reset)
     - formulario de anuncio: muestra u oculta campos según el tipo (data-show-for)
     - formulario de contrato: copia importe y moneda del plan elegido
   ===================================================================== */
(function () {
  'use strict';
  var $$ = function (sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); };

  // Menú móvil
  var toggle = document.querySelector('[data-nav-toggle]');
  var nav = document.querySelector('[data-nav]');
  if (toggle && nav) {
    toggle.addEventListener('click', function () {
      var open = nav.classList.toggle('open');
      toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
  }

  // Confirmaciones
  $$('form[data-confirm]').forEach(function (f) {
    f.addEventListener('submit', function (e) {
      if (!window.confirm(f.getAttribute('data-confirm'))) e.preventDefault();
    });
  });
  $$('[data-confirm-click]').forEach(function (b) {
    b.addEventListener('click', function (e) {
      if (!window.confirm(b.getAttribute('data-confirm-click'))) e.preventDefault();
    });
  });

  // Copiar al portapapeles el input que está al lado del botón
  $$('[data-copy]').forEach(function (b) {
    b.addEventListener('click', function () {
      var input = b.parentElement.querySelector('input');
      if (!input) return;
      input.select();
      var done = function () { b.textContent = 'Copiado'; setTimeout(function () { b.textContent = 'Copiar'; }, 1500); };
      if (navigator.clipboard) navigator.clipboard.writeText(input.value).then(done).catch(function () { document.execCommand('copy'); done(); });
      else { document.execCommand('copy'); done(); }
    });
  });

  // Colores: <input type=color> ↔ <input type=text name=...>
  $$('[data-color-for]').forEach(function (picker) {
    var text = document.querySelector('input[name="' + picker.getAttribute('data-color-for') + '"]');
    if (!text) return;
    picker.addEventListener('input', function () { text.value = picker.value.toUpperCase(); });
    text.addEventListener('input', function () { if (/^#[0-9A-Fa-f]{6}$/.test(text.value)) picker.value = text.value; });
  });
  $$('[data-color-reset]').forEach(function (a) {
    a.addEventListener('click', function (e) {
      e.preventDefault();
      var name = a.getAttribute('data-color-reset');
      var v = a.getAttribute('data-value');
      var text = document.querySelector('input[name="' + name + '"]');
      var picker = document.querySelector('[data-color-for="' + name + '"]');
      if (text) text.value = v;
      if (picker) picker.value = v;
    });
  });

  // Formulario de anuncio: campos según el tipo
  var typeSel = document.querySelector('[data-ad-type]');
  if (typeSel) {
    var sync = function () {
      $$('[data-show-for]').forEach(function (el) {
        el.hidden = el.getAttribute('data-show-for').split(' ').indexOf(typeSel.value) < 0;
      });
    };
    typeSel.addEventListener('change', sync);
    sync();
  }

  // Formulario de contrato: importe y moneda del plan
  var planSel = document.querySelector('[data-plan-select]');
  if (planSel) {
    planSel.addEventListener('change', function () {
      var opt = planSel.options[planSel.selectedIndex];
      var amount = document.querySelector('[data-amount]');
      var currency = document.querySelector('[data-currency]');
      if (opt && opt.dataset.amount && amount) amount.value = opt.dataset.amount;
      if (opt && opt.dataset.currency && currency) currency.value = opt.dataset.currency;
    });
  }
})();
