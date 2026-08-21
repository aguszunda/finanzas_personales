// Validación del lado del cliente para los formularios de autenticación.
//
// Las reglas viven en este archivo (no en atributos del HTML) para no
// exponerlas en el markup. Igual que todo chequeo client-side, esto es solo
// UX: la validación canónica y de seguridad ocurre en el servidor.
(function() {
    'use strict';

    var EMAIL_RE = /^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$/;
    var PASSWORD_RE = /^[a-zA-Z0-9]+$/;
    var MIN_PASSWORD = 8;
    var MAX_PASSWORD = 72;

    // Espejo de validatePassword (auth_service.go): longitud 8-72, solo
    // alfanumérico ASCII y al menos una letra y un número.
    function isValidPassword(pw) {
        if (pw.length < MIN_PASSWORD || pw.length > MAX_PASSWORD) return false;
        if (!PASSWORD_RE.test(pw)) return false;
        return /[a-zA-Z]/.test(pw) && /[0-9]/.test(pw);
    }

    function isAuthRequest(path) {
        return path.indexOf('/api/auth/login') !== -1 ||
            path.indexOf('/api/auth/register') !== -1;
    }

    document.addEventListener('htmx:beforeRequest', function(evt) {
        var path = (evt.detail.pathInfo && evt.detail.pathInfo.requestPath) || '';
        if (!isAuthRequest(path)) return;

        var form = evt.detail.elt;
        if (!form || !form.querySelector) return;

        var email = form.querySelector('input[name="email"]');
        if (email && !EMAIL_RE.test(email.value)) {
            evt.preventDefault();
            showAlertModal('El email no es válido. Ej: nombre@dominio.com', 'error');
            email.focus();
            return;
        }

        var isRegister = path.indexOf('/register') !== -1;
        var password = form.querySelector('input[name="password"]');
        if (isRegister && password && !isValidPassword(password.value)) {
            evt.preventDefault();
            showAlertModal('La contraseña debe tener entre ' + MIN_PASSWORD + ' y ' + MAX_PASSWORD + ' caracteres y contener solo letras y números', 'error');
            password.focus();
        }
    });
})();