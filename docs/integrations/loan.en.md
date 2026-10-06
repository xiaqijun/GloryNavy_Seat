# Loan module

Status: The shared-pool change is complete locally and awaits the next coordinated frontend/backend release; production still runs the first pool implementation. Loans use game ISK. There is one shared pool: personal or corporation contributors submit amounts to it, and the borrower is a site member account. All characters bound to one account share its loan responsibility.

The system uses fixed total interest with equal scheduled installments. Principal and interest are stored separately in integer ISK minor units; the final installment absorbs rounding. A contribution is pending until the contributor sends a completed cash-only contract to the configured personal custodian character or corporation. The amount, direction, acceptor and empty item set are checked and the global contract claim succeeds before the amount becomes pool cash. Approval is not disbursement; disbursement also reserves and debits available pool cash after contract verification.

Administrators maintain the credit score, total responsibility limit, unsecured limit, and rule version. Limits, guarantee coverage, and collateral coverage are rechecked in the approval transaction; a pool cannot approve loans while required policy values are missing. A guarantor must accept an invitation. Collateral remains pending until an administrator approves an identifiable EVE contract and item snapshot. The module never auto-debits wallets, liquidates collateral, or treats a wallet balance as payment evidence.

The shared pool's `config` must include `min_principal_minor`, `max_principal_minor`, and `max_installments`; collateral approval also requires `collateral_haircut_bps`. The pool must also configure its custodian. Unverified promises and website balances are never treated as cash. Covered collateral cannot exceed valuation multiplied by that haircut, and a submitted haircut must match the configured policy. These keys have no implicit defaults.

Payment verification uses the existing EVE contract cache service. Disbursement requires a finished contract, exact amount, and the borrower receiving character. Repayment may be partial and is allocated from the earliest unpaid installment while preserving principal and interest detail. One in-game contract can be claimed by only one delivery module; retries do not credit twice.

The module exposes the authenticated `loan.self` routes described in the Chinese guide, including contribution submission and deposit verification. Corporation contributions and custody configuration additionally check `corporation.loan`. Goose migration 71 adds the shared-pool contribution and cash ledger tables. The page appears when `loan` is enabled in `MODULES`.

Pending corporation-pool applications are also exposed as the `loan` source in the approval center. The center is read-only and reuses this module's review endpoint; personal lender decisions stay on the loan page.

Administrators can use the shared Loan configuration panel to set the sole custodian and save credit policy. Any member can submit a personal contribution; corporation contributions require the corporation permission. Members can submit applications only when an open shared pool, available verified cash, and an active credit profile exist.

Real Tranquility contract verification, direction, partial repayment, duplicate claims, and authorization expiry still require production acceptance; deployment and module enablement are not evidence that the real contract loop is accepted.
