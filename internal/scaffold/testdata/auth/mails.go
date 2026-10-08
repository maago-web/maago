package auth

import authlib "github.com/maago-web/maago/contrib/auth"

// The mails of the app. Change the words as you like; keep each link on
// a line of its own.

func verifyMail(to, link string) authlib.Message {
	return authlib.Message{To: to, Subject: "Verify your email", Body: `Welcome! To verify your email, open this link:

` + link + `

If you didn't sign up, you can ignore this email.
`}
}

func accountExistsMail(to, loginLink string) authlib.Message {
	return authlib.Message{To: to, Subject: "You already have an account", Body: `Someone tried to sign up with this email, but it already has an account. To sign in:

` + loginLink + `

If you didn't sign up, you can ignore this email.
`}
}
