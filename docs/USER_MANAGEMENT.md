# User & Account Management

AlphaDrive features a lightweight, integrated multi-user architecture with role-based access control (RBAC), secure password hashing, and in-app profile management.

---

## 1. User Roles

| Role | Privileges |
| :--- | :--- |
| **Administrator (Owner)** | - Full access to personal drive & trash<br>- Access to in-app **Add Users** tab<br>- Ability to provision new users and grant Admin role<br>- Execute system backups and diagnostics |
| **Standard User** | - Isolated personal drive and trash<br>- Create and manage personal public share links<br>- Update personal username and password |

---

## 2. First-Time Owner Setup

On initial startup with an uninitialized database:
1. Navigate to the root URL (`/`).
2. The server redirects to `/setup`.
3. Provide your Full Name, Master Username, and Password (min 12 characters).
4. Submitting creates the primary Administrator (Owner) account and permanently disables `/setup`.

---

## 3. Adding New Users (Admin Only)

Administrators can provision additional users directly from the web interface:
1. Click the **Account** avatar in the top-right header (or bottom tab on mobile).
2. Click the **Add Users** tab.
3. Enter the user details:
   - **Full Name**: (e.g. `Alice Smith`)
   - **Username**: Unique username (3â€“32 chars)
   - **Password**: Initial password (min 12 chars)
   - **Grant Administrator access**: Check to grant admin privileges.
4. Click **Create User**.

---

## 4. Updating Profile & Password

Any authenticated user can update their credentials at any time:
- **Change Username**: Switch to the **Username** tab, type the new username, and click **Save Username**.
- **Change Password**: Switch to the **Password** tab, enter current password, enter new password (min 12 chars), confirm new password, and click **Update Password**.

---

## 5. Security & Password Standards
- Passwords are hashed using **Argon2id** (standard memory cost: 64 MB, iterations: 3, parallelism: 2).
- Plaintext passwords are never logged or stored.
- Password change requires verification of the current password.
- Minimum password length is enforced at 12 characters across all forms and APIs.