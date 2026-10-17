import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

Future<bool> showLocationPermissionBlocker(BuildContext context) async =>
    await showModalBottomSheet<bool>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (sheet) => SingleChildScrollView(
        child: Padding(
          padding: EdgeInsets.fromLTRB(
            24,
            8,
            24,
            24 + MediaQuery.paddingOf(sheet).bottom,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Align(
                alignment: Alignment.centerLeft,
                child: Container(
                  padding: const EdgeInsets.all(14),
                  decoration: BoxDecoration(
                    color: sheet.palette.surfaceElevated,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: const JourneyIcon(JourneySymbol.route, size: 36),
                ),
              ),
              const SizedBox(height: 20),
              Text(
                'Let us find your position',
                style: Theme.of(sheet).textTheme.headlineSmall,
              ),
              const SizedBox(height: 10),
              Text(
                'Location access is needed to receive deliveries and share your position while online. If access is blocked, enable it in your phone settings.',
                style: Theme.of(sheet).textTheme.bodyLarge,
              ),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: () => Navigator.pop(sheet, true),
                child: const Text('Try location again'),
              ),
              TextButton(
                onPressed: () => Navigator.pop(sheet, false),
                child: const Text('Not now'),
              ),
            ],
          ),
        ),
      ),
    ) ??
    false;
