// Close menu when clicking outside
document.addEventListener('click', function(e) {
	const toggle = document.getElementById('menu-toggle');
	if (toggle && toggle.checked) {
		const header = e.target.closest('.site-header');
		if (!header) {
			toggle.checked = false;
		}
	}
});

// Close menu when pressing Escape
document.addEventListener('keydown', function(e) {
	if (e.key === 'Escape') {
		const toggle = document.getElementById('menu-toggle');
		if (toggle) toggle.checked = false;
	}
});

// Convert <time> elements to local timezone
function formatLocalTimes() {
	document.querySelectorAll('time[datetime]').forEach(el => {
		const date = new Date(el.getAttribute('datetime'));
		if (isNaN(date)) return;

		// Format as localized date string
		el.textContent = date.toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long',
			day: 'numeric'
		});
	});
}

document.addEventListener('DOMContentLoaded', formatLocalTimes);
document.body.addEventListener('htmx:afterSwap', formatLocalTimes);

// Event delegation - handle all clicks via data-action attributes
document.addEventListener('click', function(e) {
	const action = e.target.closest('[data-action]')?.dataset.action;
	if (action && handlers[action]) {
		e.preventDefault();
		handlers[action](e);
	}
});

// Action handlers
const handlers = {
	'passkey-register': registerPasskey,
	'passkey-register-account': registerPasskeyAccount,
	'passkey-login': startPasskeyLogin
};

// Initialize passkey options from data attribute when content is swapped
document.body.addEventListener('htmx:afterSwap', initPasskeyOptions);
document.addEventListener('DOMContentLoaded', initPasskeyOptions);

function initPasskeyOptions() {
	const optionsEl = document.getElementById('passkey-options');
	if (optionsEl && optionsEl.dataset.options) {
		const data = JSON.parse(optionsEl.dataset.options);
		window.passkeyOptions = {
			publicKey: {
				challenge: base64ToBuffer(data.challenge),
				rp: {
					name: data.rpName,
					id: data.rpId
				},
				user: {
					id: base64ToBuffer(data.userId),
					name: data.userName,
					displayName: data.userDisplayName
				},
				pubKeyCredParams: [
					{ type: "public-key", alg: -7 },
					{ type: "public-key", alg: -257 }
				],
				authenticatorSelection: {
					authenticatorAttachment: "platform",
					residentKey: "preferred",
					userVerification: "preferred"
				},
				timeout: 60000
			}
		};
	}
}

async function registerPasskey() {
	if (!window.passkeyOptions) {
		alert('Passkey options not loaded');
		return;
	}
	try {
		const credential = await navigator.credentials.create(window.passkeyOptions);

		const response = await fetch('/auth/passkey/register', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				id: credential.id,
				rawId: bufferToBase64(credential.rawId),
				type: credential.type,
				response: {
					attestationObject: bufferToBase64(credential.response.attestationObject),
					clientDataJSON: bufferToBase64(credential.response.clientDataJSON)
				}
			})
		});

		const result = await response.json();
		const redirect = result._links?.redirect?.href;
		if (response.ok && redirect) {
			window.location.href = redirect;
		} else {
			alert(result.error || 'Failed to register passkey');
		}
	} catch (err) {
		console.error('Passkey registration error:', err);
		if (err.name !== 'NotAllowedError') {
			alert('Failed to create passkey. Please try again.');
		}
	}
}

async function registerPasskeyAccount() {
	if (!window.passkeyOptions) {
		alert('Passkey options not loaded');
		return;
	}
	try {
		const credential = await navigator.credentials.create(window.passkeyOptions);

		const response = await fetch('/my/account/passkeys/register', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				id: credential.id,
				rawId: bufferToBase64(credential.rawId),
				type: credential.type,
				response: {
					attestationObject: bufferToBase64(credential.response.attestationObject),
					clientDataJSON: bufferToBase64(credential.response.clientDataJSON)
				}
			})
		});

		const result = await response.json();
		const redirect = result._links?.redirect?.href;
		if (response.ok && redirect) {
			window.location.href = redirect;
		} else {
			alert(result.error || 'Failed to register passkey');
		}
	} catch (err) {
		console.error('Passkey registration error:', err);
		if (err.name !== 'NotAllowedError') {
			alert('Failed to create passkey. Please try again.');
		}
	}
}

async function startPasskeyLogin() {
	// Get phone number from form - use native validation
	const phoneInput = document.getElementById('phone');
	if (!phoneInput || !phoneInput.value) {
		phoneInput?.form?.reportValidity();
		return;
	}

	try {
		const formData = new FormData();
		formData.append('phone', phoneInput.value);

		const optionsRes = await fetch('/auth/passkey/login', {
			method: 'POST',
			body: formData
		});
		if (!optionsRes.ok) {
			const err = await optionsRes.json();
			alert(err.error || 'Failed to start login');
			return;
		}
		const options = await optionsRes.json();

		options.publicKey.challenge = base64ToBuffer(options.publicKey.challenge);
		if (options.publicKey.allowCredentials) {
			options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(cred => ({
				...cred,
				id: base64ToBuffer(cred.id)
			}));
		}

		const credential = await navigator.credentials.get(options);

		const response = await fetch('/auth/passkey/login/finish', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				id: credential.id,
				rawId: bufferToBase64(credential.rawId),
				type: credential.type,
				response: {
					authenticatorData: bufferToBase64(credential.response.authenticatorData),
					clientDataJSON: bufferToBase64(credential.response.clientDataJSON),
					signature: bufferToBase64(credential.response.signature),
					userHandle: credential.response.userHandle ? bufferToBase64(credential.response.userHandle) : null
				}
			})
		});

		const result = await response.json();
		const redirect = result._links?.redirect?.href;
		if (response.ok && redirect) {
			window.location.href = redirect;
		} else {
			alert(result.error || 'Login failed');
		}
	} catch (err) {
		console.error('Passkey login error:', err);
		if (err.name !== 'NotAllowedError') {
			alert('Failed to use passkey. Please try SMS verification.');
		}
	}
}

function base64ToBuffer(base64) {
	const binary = atob(base64.replace(/-/g, '+').replace(/_/g, '/'));
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) {
		bytes[i] = binary.charCodeAt(i);
	}
	return bytes.buffer;
}

function bufferToBase64(buffer) {
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (let i = 0; i < bytes.length; i++) {
		binary += String.fromCharCode(bytes[i]);
	}
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}
