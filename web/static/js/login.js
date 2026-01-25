/**
 * Login Page - Client-side validation and interactions
 */

(function() {
    'use strict';

    // DOM Elements
    const loginForm = document.getElementById('loginForm');
    const emailInput = document.getElementById('email');
    const passwordInput = document.getElementById('password');
    const emailError = document.getElementById('emailError');
    const passwordError = document.getElementById('passwordError');
    const togglePasswordBtn = document.getElementById('togglePassword');
    const loginButton = document.getElementById('loginButton');
    const buttonText = loginButton.querySelector('.button-text');
    const buttonLoader = loginButton.querySelector('.button-loader');

    // Validation patterns
    const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    const passwordMinLength = 6;

    /**
     * Show error message for a field
     */
    function showError(input, errorElement, message) {
        input.classList.add('error');
        errorElement.textContent = message;
        errorElement.classList.add('show');
    }

    /**
     * Clear error message for a field
     */
    function clearError(input, errorElement) {
        input.classList.remove('error');
        errorElement.textContent = '';
        errorElement.classList.remove('show');
    }

    /**
     * Validate email field
     */
    function validateEmail() {
        const email = emailInput.value.trim();

        if (!email) {
            showError(emailInput, emailError, 'Email address is required');
            return false;
        }

        if (!emailPattern.test(email)) {
            showError(emailInput, emailError, 'Please enter a valid email address');
            return false;
        }

        clearError(emailInput, emailError);
        return true;
    }

    /**
     * Validate password field
     */
    function validatePassword() {
        const password = passwordInput.value;

        if (!password) {
            showError(passwordInput, passwordError, 'Password is required');
            return false;
        }

        if (password.length < passwordMinLength) {
            showError(passwordInput, passwordError, `Password must be at least ${passwordMinLength} characters`);
            return false;
        }

        clearError(passwordInput, passwordError);
        return true;
    }

    /**
     * Toggle password visibility
     */
    function togglePasswordVisibility() {
        const type = passwordInput.type === 'password' ? 'text' : 'password';
        passwordInput.type = type;

        // Update icon (optional - you can add different SVG for visible state)
        const eyeIcon = togglePasswordBtn.querySelector('.eye-icon');
        eyeIcon.style.opacity = type === 'text' ? '0.7' : '1';
    }

    /**
     * Handle form submission
     */
    async function handleSubmit(event) {
        event.preventDefault();

        // Validate all fields
        const isEmailValid = validateEmail();
        const isPasswordValid = validatePassword();

        if (!isEmailValid || !isPasswordValid) {
            // Focus on first invalid field
            if (!isEmailValid) {
                emailInput.focus();
            } else if (!isPasswordValid) {
                passwordInput.focus();
            }
            return;
        }

        // Show loading state
        loginButton.disabled = true;
        buttonText.style.display = 'none';
        buttonLoader.style.display = 'flex';

        try {
            // Prepare login data
            const loginData = {
                email: emailInput.value.trim(),
                password: passwordInput.value,
                remember: document.getElementById('remember').checked
            };

            // Send login request
            const response = await fetch('/api/v1/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                credentials: 'include', // Important for cookies
                body: JSON.stringify(loginData)
            });

            const data = await response.json();

            if (response.ok) {
                // Success - show success message briefly then redirect
                showSuccess('Login successful! Redirecting...');
                
                // Redirect after a brief delay
                setTimeout(() => {
                    window.location.href = data.redirect || '/profile';
                }, 500);
            } else {
                // Handle different error scenarios
                let errorMessage = data.error || 'Login failed';
                
                if (response.status === 401) {
                    errorMessage = 'Invalid email or password';
                } else if (response.status === 403) {
                    errorMessage = 'Account is deactivated. Contact administrator.';
                } else if (response.status === 400) {
                    errorMessage = data.error || 'Invalid input';
                }

                showError(emailInput, emailError, errorMessage);
                resetButtonState();
                
                // Clear password field for security
                passwordInput.value = '';
                passwordInput.focus();
            }
        } catch (error) {
            console.error('Login error:', error);
            showError(emailInput, emailError, 'Network error. Please check your connection and try again.');
            resetButtonState();
        }
    }

    /**
     * Show success message
     */
    function showSuccess(message) {
        // Create or get success message element
        let successMsg = document.querySelector('.success-message');
        if (!successMsg) {
            successMsg = document.createElement('div');
            successMsg.className = 'success-message';
            successMsg.style.cssText = 'color: #10b981; font-size: 14px; margin-top: 10px; text-align: center;';
            loginButton.parentElement.appendChild(successMsg);
        }
        successMsg.textContent = message;
        successMsg.style.display = 'block';
    }

    /**
     * Reset button to normal state
     */
    function resetButtonState() {
        loginButton.disabled = false;
        buttonText.style.display = 'inline';
        buttonLoader.style.display = 'none';
    }

    /**
     * Handle real-time validation on input
     */
    function handleInput(input, validateFn) {
        // Only validate if field has been touched and has value
        if (input.value.length > 0) {
            // Debounce validation
            clearTimeout(input.validationTimeout);
            input.validationTimeout = setTimeout(() => {
                validateFn();
            }, 500);
        }
    }

    /**
     * Initialize event listeners
     */
    function init() {
        // Form submission
        loginForm.addEventListener('submit', handleSubmit);

        // Email validation on blur
        emailInput.addEventListener('blur', validateEmail);
        
        // Password validation on blur
        passwordInput.addEventListener('blur', validatePassword);

        // Real-time validation on input (with debounce)
        emailInput.addEventListener('input', () => {
            // Clear error while typing
            if (emailInput.classList.contains('error')) {
                handleInput(emailInput, validateEmail);
            }
        });

        passwordInput.addEventListener('input', () => {
            // Clear error while typing
            if (passwordInput.classList.contains('error')) {
                handleInput(passwordInput, validatePassword);
            }
        });

        // Toggle password visibility
        if (togglePasswordBtn) {
            togglePasswordBtn.addEventListener('click', togglePasswordVisibility);
        }

        // Handle "Enter" key on inputs
        emailInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                if (validateEmail()) {
                    passwordInput.focus();
                }
            }
        });

        passwordInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                handleSubmit(e);
            }
        });

        // Auto-focus email input on page load
        emailInput.focus();

        // Check for stored error messages (from server-side validation)
        const urlParams = new URLSearchParams(window.location.search);
        const errorMsg = urlParams.get('error');
        if (errorMsg) {
            showError(emailInput, emailError, decodeURIComponent(errorMsg));
        }

        // Clear URL parameters after displaying error
        if (errorMsg) {
            window.history.replaceState({}, document.title, window.location.pathname);
        }
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Prevent form resubmission on page refresh
    if (window.history.replaceState) {
        window.history.replaceState(null, null, window.location.href);
    }

})();
