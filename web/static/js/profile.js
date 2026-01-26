/**
 * Profile Page - Client-side functionality
 */

(function() {
    'use strict';

    // DOM Elements
    const profileForm = document.getElementById('profileForm');
    const passwordForm = document.getElementById('passwordForm');
    const tabBtns = document.querySelectorAll('.tab-btn');
    const logoutBtn = document.getElementById('logoutBtn');

    // Profile form elements
    const usernameInput = document.getElementById('username');
    const fullNameInput = document.getElementById('fullName');
    const emailInput = document.getElementById('email');
    const updateProfileBtn = document.getElementById('updateProfileBtn');
    const cancelProfileBtn = document.getElementById('cancelProfileBtn');

    // Password form elements
    const currentPasswordInput = document.getElementById('currentPassword');
    const newPasswordInput = document.getElementById('newPassword');
    const confirmPasswordInput = document.getElementById('confirmPassword');
    const changePasswordBtn = document.getElementById('changePasswordBtn');
    const cancelPasswordBtn = document.getElementById('cancelPasswordBtn');

    // Display elements
    const userName = document.getElementById('userName');
    const userEmail = document.getElementById('userEmail');
    const userRole = document.getElementById('userRole');
    const avatarInitials = document.getElementById('avatarInitials');

    // Store original user data
    let originalUserData = {};

    /**
     * Show error message for a field
     */
    function showError(input, errorElementId, message) {
        const errorElement = document.getElementById(errorElementId);
        input.classList.add('error');
        errorElement.textContent = message;
    }

    /**
     * Clear error message for a field
     */
    function clearError(input, errorElementId) {
        const errorElement = document.getElementById(errorElementId);
        input.classList.remove('error');
        errorElement.textContent = '';
    }

    /**
     * Clear all errors in a form
     */
    function clearAllErrors(form) {
        const inputs = form.querySelectorAll('input');
        inputs.forEach(input => {
            input.classList.remove('error');
        });
        const errorMessages = form.querySelectorAll('.error-message');
        errorMessages.forEach(msg => msg.textContent = '');
    }

    /**
     * Show alert message
     */
    function showAlert(message, type = 'success') {
        // Remove existing alerts
        const existingAlerts = document.querySelectorAll('.alert');
        existingAlerts.forEach(alert => alert.remove());

        const alert = document.createElement('div');
        alert.className = `alert alert-${type}`;
        alert.innerHTML = `
            <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
                ${type === 'success' 
                    ? '<path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"/>'
                    : '<path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z"/>'}
            </svg>
            <span>${message}</span>
        `;

        const activeTab = document.querySelector('.tab-pane.active');
        activeTab.insertBefore(alert, activeTab.firstChild);

        // Auto remove after 5 seconds
        setTimeout(() => alert.remove(), 5000);
    }

    /**
     * Toggle button loading state
     */
    function toggleButtonLoading(button, isLoading) {
        const btnText = button.querySelector('.btn-text');
        const btnLoader = button.querySelector('.btn-loader');
        
        if (isLoading) {
            button.disabled = true;
            btnText.style.display = 'none';
            btnLoader.style.display = 'flex';
        } else {
            button.disabled = false;
            btnText.style.display = 'inline';
            btnLoader.style.display = 'none';
        }
    }

    /**
     * Load user profile data
     */
    async function loadProfile() {
        try {
            const response = await fetch('/api/v1/profile', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            if (!response.ok) {
                if (response.status === 401) {
                    // Not authenticated, redirect to login
                    window.location.href = '/login';
                    return;
                }
                throw new Error('Failed to load profile');
            }

            const data = await response.json();
            
            // Store original data
            originalUserData = data;

            // Update display
            userName.textContent = data.full_name?.String || data.username;
            userEmail.textContent = data.email;
            userRole.textContent = data.role;
            
            // Update avatar initials
            const initials = getInitials(data.full_name?.String || data.username);
            avatarInitials.textContent = initials;

            // Populate form
            usernameInput.value = data.username;
            fullNameInput.value = data.full_name?.String || '';
            emailInput.value = data.email;

        } catch (error) {
            console.error('Failed to load profile:', error);
            showAlert('Failed to load profile data. Please refresh the page.', 'error');
        }
    }

    /**
     * Get initials from name
     */
    function getInitials(name) {
        if (!name) return 'U';
        const parts = name.trim().split(' ');
        if (parts.length >= 2) {
            return (parts[0][0] + parts[1][0]).toUpperCase();
        }
        return name.substring(0, 2).toUpperCase();
    }

    /**
     * Handle profile update
     */
    async function handleProfileUpdate(event) {
        event.preventDefault();
        clearAllErrors(profileForm);

        // Validate
        let isValid = true;
        
        if (!usernameInput.value.trim()) {
            showError(usernameInput, 'usernameError', 'Username is required');
            isValid = false;
        }

        if (!emailInput.value.trim()) {
            showError(emailInput, 'emailError', 'Email is required');
            isValid = false;
        }

        if (!isValid) return;

        toggleButtonLoading(updateProfileBtn, true);

        try {
            const updateData = {
                username: usernameInput.value.trim(),
                full_name: fullNameInput.value.trim(),
                email: emailInput.value.trim()
            };

            const response = await fetch('/api/v1/profile', {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                credentials: 'include',
                body: JSON.stringify(updateData)
            });

            const data = await response.json();

            if (response.ok) {
                showAlert('Profile updated successfully!', 'success');
                // Reload profile data
                await loadProfile();
            } else {
                showAlert(data.error || 'Failed to update profile', 'error');
            }
        } catch (error) {
            console.error('Profile update error:', error);
            showAlert('Network error. Please try again.', 'error');
        } finally {
            toggleButtonLoading(updateProfileBtn, false);
        }
    }

    /**
     * Handle password change
     */
    async function handlePasswordChange(event) {
        event.preventDefault();
        clearAllErrors(passwordForm);

        // Validate
        let isValid = true;

        if (!currentPasswordInput.value) {
            showError(currentPasswordInput, 'currentPasswordError', 'Current password is required');
            isValid = false;
        }

        if (!newPasswordInput.value) {
            showError(newPasswordInput, 'newPasswordError', 'New password is required');
            isValid = false;
        } else if (newPasswordInput.value.length < 8) {
            showError(newPasswordInput, 'newPasswordError', 'Password must be at least 8 characters');
            isValid = false;
        }

        if (!confirmPasswordInput.value) {
            showError(confirmPasswordInput, 'confirmPasswordError', 'Please confirm your password');
            isValid = false;
        } else if (newPasswordInput.value !== confirmPasswordInput.value) {
            showError(confirmPasswordInput, 'confirmPasswordError', 'Passwords do not match');
            isValid = false;
        }

        if (!isValid) return;

        toggleButtonLoading(changePasswordBtn, true);

        try {
            const passwordData = {
                current_password: currentPasswordInput.value,
                new_password: newPasswordInput.value,
                confirm_password: confirmPasswordInput.value
            };

            const response = await fetch('/api/v1/profile/change-password', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                credentials: 'include',
                body: JSON.stringify(passwordData)
            });

            const data = await response.json();

            if (response.ok) {
                showAlert('Password changed successfully!', 'success');
                passwordForm.reset();
            } else {
                showAlert(data.error || 'Failed to change password', 'error');
            }
        } catch (error) {
            console.error('Password change error:', error);
            showAlert('Network error. Please try again.', 'error');
        } finally {
            toggleButtonLoading(changePasswordBtn, false);
        }
    }

    /**
     * Handle tab switching
     */
    function switchTab(tabBtn) {
        // Remove active class from all tabs and panes
        tabBtns.forEach(btn => btn.classList.remove('active'));
        document.querySelectorAll('.tab-pane').forEach(pane => pane.classList.remove('active'));

        // Add active class to clicked tab
        tabBtn.classList.add('active');

        // Show corresponding tab pane
        const tabId = tabBtn.getAttribute('data-tab');
        const tabPane = document.getElementById(tabId);
        if (tabPane) {
            tabPane.classList.add('active');
        }

        // Clear alerts and errors
        clearAllErrors(profileForm);
        clearAllErrors(passwordForm);
        document.querySelectorAll('.alert').forEach(alert => alert.remove());
    }

    /**
     * Handle logout
     */
    function handleLogout() {
        // TODO: Call logout API endpoint
        // For now, just redirect to login
        localStorage.clear();
        sessionStorage.clear();
        window.location.href = '/login';
    }

    /**
     * Reset profile form
     */
    function resetProfileForm() {
        usernameInput.value = originalUserData.username;
        fullNameInput.value = originalUserData.full_name?.String || '';
        emailInput.value = originalUserData.email;
        clearAllErrors(profileForm);
    }

    /**
     * Reset password form
     */
    function resetPasswordForm() {
        passwordForm.reset();
        clearAllErrors(passwordForm);
    }

    /**
     * Initialize event listeners
     */
    function init() {
        // Load profile data on page load
        loadProfile();

        // Profile form submission
        profileForm.addEventListener('submit', handleProfileUpdate);
        cancelProfileBtn.addEventListener('click', resetProfileForm);

        // Password form submission
        passwordForm.addEventListener('submit', handlePasswordChange);
        cancelPasswordBtn.addEventListener('click', resetPasswordForm);

        // Tab switching
        tabBtns.forEach(btn => {
            btn.addEventListener('click', () => switchTab(btn));
        });

        // Logout
        logoutBtn.addEventListener('click', handleLogout);

        // Clear errors on input
        [usernameInput, fullNameInput, emailInput].forEach(input => {
            input.addEventListener('input', () => {
                const errorId = input.id + 'Error';
                clearError(input, errorId);
            });
        });

        [currentPasswordInput, newPasswordInput, confirmPasswordInput].forEach(input => {
            input.addEventListener('input', () => {
                const errorId = input.id + 'Error';
                clearError(input, errorId);
            });
        });
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
