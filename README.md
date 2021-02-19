# Picocrypt
Picocrypt is a <i>very tiny</i> (hence "Pico"), very simple, yet very secure file encryption tool. It uses the modern XChaCha20-Poly1305 cipher suite as well as Argon2ID, making it about as secure and modern of an encryption tool as you'll ever get your hands on. Picocrypt's focus is <i>security</i>, so it might be slightly slower than others. 

# Download
You can run the raw Python source file, compile it yourself, or download the portable .exe (for Windows) that I've precompiled and optimized beyond imagination (recommended, because it's just 4MB in size) <a href="https://github.com/HACKERALERT/Picocrypt/raw/main/Picocrypt.exe" target="_blank">here</a>. If you're compiling from source or running the raw Python file, you'll need these two <code>pip</code> dependencies: <code>argon2-cffi</code> and <code>pycryptodome</code>.

# Why Picocrypt?
Why should you use Picocrypt, instead of Bitlocker, NordLocker, VeraCrypt, or 7-Zip? Here are some reasons why you should switch to Picocrypt:

<ul>
	<li>Unlike NordLocker and Bitlocker, Picocrypt is FOSS (free open-source software) and can be audited. You can verify for yourself that there aren't any backdoors.</li>
	<li>Picocrypt is portable and <i>tiny</i> (just 4MB!). It's much lighter than NordLocker (>100MB) and VeraCrypt (>30MB). It can also run on any machine (since it's Python) and the pre-made .exe can run on any Windows PC from 7 and up.</li>
	<li>It's infinitely easier to use than VeraCrypt (no need to create volumes) and a 5-year-old could use Picocrypt.</li>
	<li>Picocrypt is built for security, using modern standards and the most secure settings. See <strong>Security</strong> below for more info.</li>
	<li>It supports file integrity checking through Poly1305, which means that you would know if a hacker has maliciously modified your data.</li>
</ul>

# Instructions
Picocrypt is about as simple as it gets. Just select a file, enter a password, and start. There are some additional options that you can use for more control:

<ul>
	<li>File metadata (editable for encryption, readonly for decryption): Use this to store notes, information, and text along with the file (it won't be encrypted). For example, you can put a description of the file before sending it to someone. When the person you sent it to selects the file in Picocrypt, your description will be shown to that person.</li>
	<li>Keep decrypted output even if it's corrupted or modified (decryption only): Picocrypt automatically checks for integrity upon decryption. If the file has been modified or is corrupted, Picocrypt will delete the output. If you want to keep the corrupt or modified data after decryption, check this option.</li>
	<li>Securely erase and delete original file (encryption only): If checked, Picocrypt will generate pseudo-random data and write it to the original file while encrypting, effectively wiping the original file. The file will be deleted once encryption is complete.</li>
</ul>

# Security
Security is Picocrypt's sole focus. I was in need of a secure, reliable, and future-proof encryption tool that didn't require bloatware and containers, but I couldn't find one, so I created Picocrypt. Picocrypt uses XChaCha20-Poly1305, which is a revision of the eSTREAM winner, Salsa20. XChaCha20-Poly1305 has been through significant amount of cryptanalysis and was selected by security engineers at Google to be used in modern TLS suites. It's considered to be the future of encryption, and makes Picocrypt more secure than Bitlocker, NordLocker, 7-Zip, and VeraCrypt. For key derivation, Picocrypt uses Argon2ID, winner of the PHC (Password Hashing Competition), which was completed in 2015. Both XChaCha20-Poly1305 and Argon2ID are well recognized within the cryptography community and both are mature and future-proof. Let me get this clear: <i>I did not write the crypto for Picocrypt</i>. Instead, I followed cryptography's number one rule: <i>Don't roll your own crypto</i>. Picocrypt uses two Python libraries, <code>argon2-cffi</code> and <code>pycryptodome</code>, both of which are well known and popular within the Python community. For people who want to know how Picocrypt handles the crypto, or for the paranoid, here is a breakdown of how Picocrypt protects your data:

<ol>
	<li>A 16-byte salt (for Argon2ID) and a 24-byte nonce (for XChaCha20) is generated using a CSPRNG (Python's <code>os.urandom()</code>)</li>
	<li>
		The encryption/decryption key is generated through Argon2ID using the salt above and the following parameters:
		<ul>
			<li>Time cost: 16</li>
			<li>Memory cost: 2^30 (1GB)</li>
			<li>Parallelism: 4</li>
		</ul>
	</li>
	<li>If decryption, compare the derived key with the SHA3_512 hash stored in the ciphertext. If encrypting, compute the SHA3_512 of the derived key and add to ciphertext.</li>
	<li>Encryption/decryption start, reading in 1MB chunks at a time. For each chunk, it is first encrypted by XChaCha20, and then CRC (SHA3_512) is updated.</li>
	<li>If 'Secure wipe' is enabled, 1MB of CSPRNG data is written to the original file.</li>
	<li>When encryption/decryption is finished, the MAC tag (Poly1305) will be added to the ciphertext or verified, depending on if you're encrypting or decrypting.</li>
	<li>Similar to above, the CRC is checked or added to the ciphertext.</li>
</ol>
