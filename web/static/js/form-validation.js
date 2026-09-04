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
            path.indexOf('/api/auth/register') !== -1 ||
            path.indexOf('/api/auth/reset-password') !== -1 ||
            path.indexOf('/api/profile/password') !== -1;
    }

    // Cancela el request y revierte el .loading que el listener global de
    // layout.html agregó en beforeRequest: al prevenir el evento htmx no
    // dispara afterRequest, así que sin esto la página queda opacada.
    function cancelRequest(evt) {
        evt.preventDefault();
        var target = evt.detail.target;
        if (target) target.classList.remove('loading');
    }

    document.addEventListener('htmx:beforeRequest', function(evt) {
        var path = (evt.detail.pathInfo && evt.detail.pathInfo.requestPath) || '';
        if (!isAuthRequest(path)) return;

        var form = evt.detail.elt;
        if (!form || !form.querySelector) return;

        var email = form.querySelector('input[name="email"]');
        if (email && !EMAIL_RE.test(email.value)) {
            cancelRequest(evt);
            showAlertModal('El email no es válido. Ej: nombre@dominio.com', 'error');
            email.focus();
            return;
        }

        var isLogin = path.indexOf('/api/auth/login') !== -1;
        var isReset = path.indexOf('/reset-password') !== -1;
        var isChange = path.indexOf('/profile/password') !== -1;

        // El campo que guarda la nueva contraseña cambia según el formulario:
        // register/reset usan "password"; cambiar contraseña usa "password_nuevo".
        // En login no se valida la fortaleza (existen cuentas con reglas viejas).
        var password = isChange
            ? form.querySelector('input[name="password_nuevo"]')
            : form.querySelector('input[name="password"]');
        var password2 = form.querySelector('input[name="password2"]');

        if (!isLogin && password && !isValidPassword(password.value)) {
            cancelRequest(evt);
            password.value = '';
            if (password2) password2.value = '';
            showAlertModal('La contraseña debe tener entre ' + MIN_PASSWORD + ' y ' + MAX_PASSWORD + ' caracteres, contener solo letras y números y no incluir espacios ni caracteres especiales', 'error');
            password.focus();
            return;
        }

        if (isChange) {
            var passwordActual = form.querySelector('input[name="password_actual"]');
            if (password && passwordActual && password.value === passwordActual.value) {
                cancelRequest(evt);
                password.value = '';
                if (password2) password2.value = '';
                showAlertModal('La nueva contraseña debe ser distinta a la actual', 'error');
                password.focus();
                return;
            }
        }

        if (isReset || isChange) {
            if (password && password2 && password.value !== password2.value) {
                cancelRequest(evt);
                password2.value = '';
                showAlertModal('Las contraseñas no coinciden', 'error');
                password2.focus();
                return;
            }
        }
    });
})();