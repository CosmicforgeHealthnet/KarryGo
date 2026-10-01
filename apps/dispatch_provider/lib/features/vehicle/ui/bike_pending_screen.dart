import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

class BikePendingScreen extends StatelessWidget {
  const BikePendingScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 24, 24, 24),
          child: Column(
            children: [
              const Spacer(),
              Container(
                width: 88,
                height: 88,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: c.info.withValues(alpha: 0.12),
                ),
                child: Icon(
                  Icons.schedule_rounded,
                  size: 44,
                  color: c.info,
                ),
              ),
              const SizedBox(height: 24),
              Text(
                'Under review',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 12),
              Text(
                "We're reviewing your bike. You'll be notified once it's approved.",
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                  height: 1.5,
                ),
              ),
              const Spacer(),
              SizedBox(
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
                  child: const Text('Back to vehicles'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}