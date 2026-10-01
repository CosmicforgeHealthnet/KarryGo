import 'package:flutter/material.dart';

import '../../../core/theme/dispatch_colors.dart';

class PeriodFilterSheet extends StatefulWidget {
  const PeriodFilterSheet({
    super.key,
    required this.selected,
    required this.onSelect,
  });

  final String selected;
  final ValueChanged<String> onSelect;

  /// Opens the sheet. Returns the selected period, or null if closed.
  static Future<String?> show(
    BuildContext context, {
    required String selected,
  }) {
    return showModalBottomSheet<String>(
      context: context,
      backgroundColor: DispatchColors.brandWhite,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(24)),
      ),
      builder: (_) => PeriodFilterSheet(
        selected: selected,
        onSelect: (v) => Navigator.of(context).pop(v),
      ),
    );
  }

  @override
  State<PeriodFilterSheet> createState() => _PeriodFilterSheetState();
}

class _PeriodFilterSheetState extends State<PeriodFilterSheet> {
  static const _options = [
    'This week',
    'This month',
    'Last 3 months',
    'This year',
    'Custom range',
  ];

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 12),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Drag handle
            Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: DispatchColors.navy20,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            const SizedBox(height: 18),

            // Title row: Earnings · X
            Row(
              children: [
                const Expanded(
                  child: Text(
                    'Earnings',
                    style: TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                      color: DispatchColors.brandNavy900,
                      letterSpacing: -0.4,
                    ),
                  ),
                ),
                GestureDetector(
                  onTap: () => Navigator.of(context).pop(),
                  behavior: HitTestBehavior.opaque,
                  child: const Padding(
                    padding: EdgeInsets.all(4),
                    child: Icon(
                      Icons.close_rounded,
                      size: 22,
                      color: DispatchColors.brandNavy900,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),

            // Options
            ..._options.map((option) {
              final isSelected = option == widget.selected;
              return GestureDetector(
                onTap: () => widget.onSelect(option),
                behavior: HitTestBehavior.opaque,
                child: Container(
                  margin: const EdgeInsets.only(bottom: 8),
                  padding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 15,
                  ),
                  decoration: BoxDecoration(
                    color: DispatchColors.brandWhite,
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(
                      color: isSelected
                          ? DispatchColors.brandTeal500
                          : DispatchColors.navy20,
                      width: isSelected ? 1.5 : 1,
                    ),
                  ),
                  child: Row(
                    children: [
                      Expanded(
                        child: Text(
                          option,
                          style: TextStyle(
                            fontSize: 15,
                            fontWeight: isSelected
                                ? FontWeight.w700
                                : FontWeight.w500,
                            color: isSelected
                                ? DispatchColors.brandNavy900
                                : DispatchColors.brandNavy900,
                          ),
                        ),
                      ),
                      if (isSelected)
                        Container(
                          width: 20,
                          height: 20,
                          decoration: const BoxDecoration(
                            color: DispatchColors.brandTeal500,
                            shape: BoxShape.circle,
                          ),
                          child: const Icon(
                            Icons.check_rounded,
                            size: 13,
                            color: Colors.white,
                          ),
                        ),
                    ],
                  ),
                ),
              );
            }),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }
}