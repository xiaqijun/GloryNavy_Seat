# Loan module

Status: The shared pool is published. Loans use game ISK. There is one fixed shared pool: personal or corporation contributors submit amounts to it, and the borrower is a site member account. All characters bound to one account share its loan responsibility.

The system uses fixed total interest with equal scheduled installments. Principal and interest are stored separately in integer ISK minor units; the final installment absorbs rounding. A contribution is pending until the contributor sends a completed cash-only contract to the configured personal custodian character or corporation. The amount, direction, acceptor and empty item set are checked and the global contract claim succeeds before the amount becomes pool cash. Approval is not disbursement; disbursement also reserves and debits available pool cash after contract verification.

The system evaluates the credit score, total responsibility limit, unsecured limit, and rule version from the borrower's loan repayment history; administrators cannot edit these values. The evaluation uses settled loans, paid installments, current responsibility, overdue installments, and defaults, and runs again when the loan page is read and inside the approval transaction. A default suspends the credit state. Guarantee and collateral coverage are also rechecked in the approval transaction. A guarantor must accept an invitation. Collateral remains pending until an administrator approves an identifiable EVE contract and item snapshot. The module never auto-debits wallets, liquidates collateral, or treats a wallet balance as payment evidence.

The shared pool is a fixed singleton. The page does not expose pool creation, switching, or pool limit configuration; members submit amounts only. The deployed pool record determines its custodian for deposit verification. Unverified promises and website balances are never treated as cash. Covered collateral cannot exceed valuation multiplied by the published haircut policy, and a submitted haircut must match that policy.

Payment verification uses the existing EVE contract cache service. Disbursement requires a finished contract, exact amount, and the borrower receiving character. Repayment may be partial and is allocated from the earliest unpaid installment while preserving principal and interest detail. One in-game contract can be claimed by only one delivery module; retries do not credit twice.

The module exposes the authenticated `loan.self` routes described in the Chinese guide, including contribution submission and deposit verification. Corporation contributions and custody configuration additionally check `corporation.loan`. Goose migration 71 adds the shared-pool contribution and cash ledger tables. The page appears when `loan` is enabled in `MODULES`.

Pending corporation-pool applications are also exposed as the `loan` source in the approval center. The center is read-only and reuses this module's review endpoint; personal lender decisions stay on the loan page.

The loan page has no shared-pool or manual credit configuration panel. The pool is a fixed singleton; contributions and loan applications open in form dialogs. The page displays the system evaluation and its derived limits. Corporation contributions require the corporation permission. Approval re-evaluates credit, cash, guarantees, and collateral before accepting an application.

Real Tranquility contract verification, direction, partial repayment, duplicate claims, and authorization expiry still require production acceptance; deployment and module enablement are not evidence that the real contract loop is accepted.
