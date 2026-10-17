import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../../../core/theme/dispatch_colors.dart';
import '../mock/wallet_tab_mock.dart';

class SetDefaultAccountScreen extends StatelessWidget {
  const SetDefaultAccountScreen({super.key, required this.account});

  final BankAccount account;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: DispatchColors.brandWhite,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            children: [
              const SizedBox(height: 8),
              Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: const Padding(
                      padding: EdgeInsets.all(6),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: DispatchColors.brandNavy900,
                      ),
                    ),
                  ),
                  const Spacer(),
                  const Text(
                    'Set default account',
                    style: TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                      color: DispatchColors.brandNavy900,
                    ),
                  ),
                  const Spacer(),
                  const SizedBox(width: 34),
                ],
              ),
              const Spacer(),

              Container(
                width: 88,
                height: 88,
                decoration: const BoxDecoration(
                  color: Color(0xFFD1FAE5),
                  shape: BoxShape.circle,
                ),
                child: const Icon(
                  Icons.check_rounded,
                  size: 46,
                  color: Color(0xFF0E9F6E),
                ),
              ),
              const SizedBox(height: 22),

              const Text(
                'Default account set',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w700,
                  color: DispatchColors.brandNavy900,
                  letterSpacing: -0.4,
                  height: 1.2,
                ),
              ),
              const SizedBox(height: 12),

              const Text(
                'This account is now your default withdrawal account.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w400,
                  color: DispatchColors.navy60,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 32),

              _AccountCard(account: account),

              const Spacer(),
            ],
          ),
        ),
      ),
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
          child: SizedBox(
            width: double.infinity,
            height: 56,
            child: FilledButton(
              onPressed: () => Navigator.of(context).pop(),
              style: FilledButton.styleFrom(
                backgroundColor: DispatchColors.brandTeal500,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(14),
                ),
              ),
              child: const Text(
                'Done',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF081F44),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _AccountCard extends StatelessWidget {
  const _AccountCard({required this.account});
  final BankAccount account;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: DispatchColors.brandWhite,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: DispatchColors.navy20, width: 1),
      ),
      child: Row(
        children: [
          Container(
            width: 44,
            height: 44,
            decoration: const BoxDecoration(
              color: Color(0xFFFFF3E0),
              shape: BoxShape.circle,
            ),
            child: const JourneyIcon(
              JourneySymbol.bank,
              size: 22,
              color: Color(0xFFF59E0B),
            ),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  account.bankName,
                  style: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: DispatchColors.brandNavy900,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  account.accountNumber,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: DispatchColors.navy60,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
