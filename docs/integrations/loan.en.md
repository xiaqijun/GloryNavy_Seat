# Loan module

Status: local first implementation completed on 2026-10-06; not released to production. Loans use game ISK. A lender is either an explicitly enabled personal pool or a corporation pool, and the borrower is a site member account. All characters bound to one account share its loan responsibility.

The first release uses fixed total interest with equal scheduled installments. Principal and interest are stored separately in integer ISK minor units; the final installment absorbs rounding. Approval is not disbursement. The loan becomes active only after a completed in-game contract is read, the amount and participant character are checked, and the global contract claim succeeds.

Administrators maintain the credit score, total responsibility limit, unsecured limit, and rule version. Limits, guarantee coverage, and collateral coverage are rechecked in the approval transaction; a pool cannot approve loans while required policy values are missing. A guarantor must accept an invitation. Collateral remains pending until an administrator approves an identifiable EVE contract and item snapshot. The module never auto-debits wallets, liquidates collateral, or treats a wallet balance as payment evidence.

A pool's `config` must include `min_principal_minor`, `max_principal_minor`, and `max_installments`; collateral approval also requires `collateral_haircut_bps`. Covered collateral cannot exceed valuation multiplied by that haircut, and a submitted haircut must match the configured policy. These keys have no implicit defaults.

Payment verification uses the existing EVE contract cache service. Disbursement requires a finished contract, exact amount, and the borrower receiving character. Repayment may be partial and is allocated from the earliest unpaid installment while preserving principal and interest detail. One in-game contract can be claimed by only one delivery module; retries do not credit twice.

The module exposes the authenticated `loan.self` routes described in the Chinese guide. Corporation pool management additionally checks `corporation.loan`. Goose migration 70 creates the private loan tables. The page appears only when `loan` is enabled in `MODULES`.

Pending corporation-pool applications are also exposed as the `loan` source in the approval center. The center is read-only and reuses this module's review endpoint; personal lender decisions stay on the loan page.

Real Tranquility contract verification, direction, partial repayment, duplicate claims, and authorization expiry still require production acceptance; a local build is not a production release.
