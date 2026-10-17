import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

class BikeEligibilityScreen extends StatelessWidget {
  const BikeEligibilityScreen({
    super.key,
    required this.vehicleController,
    required this.vehicleId,
  });

  final dynamic vehicleController;
  final String vehicleId;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(8),
                      child: Icon(
                        Icons.arrow_back_rounded,
                        size: 22,
                        color: c.text,
                      ),
                    ),
                  ),
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  Center(
                    child: Container(
                      width: 96,
                      height: 96,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: c.successSoft,
                      ),
                      child: Icon(
                        Icons.check_rounded,
                        size: 54,
                        color: c.success,
                      ),
                    ),
                  ),
                  const SizedBox(height: 24),

                  Container(
                    alignment: Alignment.center,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 14,
                        vertical: 6,
                      ),
                      decoration: BoxDecoration(
                        color: c.successSoft,
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: Text(
                        'Eligible',
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: c.success,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 16),

                  Text(
                    'Your bike meets Karryam requirements',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                      letterSpacing: -0.3,
                    ),
                  ),
                  const SizedBox(height: 32),

                  const _Check(text: 'Valid documents'),
                  const SizedBox(height: 18),
                  const _Check(text: 'Bike specification'),
                  const SizedBox(height: 18),
                  const _Check(text: 'Insurance (if required)'),
                  const SizedBox(height: 18),
                  const _Check(text: 'Meets platform standards'),
                ],
              ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => Navigator.of(context).pop(),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('View details'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Check extends StatelessWidget {
  const _Check({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Icon(Icons.check_circle_rounded, size: 22, color: c.success),
        const SizedBox(width: 14),
        Expanded(
          child: Text(
            text,
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w500,
              color: c.text,
            ),
          ),
        ),
      ],
    );
  }
}
