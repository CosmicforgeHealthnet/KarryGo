import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';

/// Public tab enum shared between the Activity screen and the filter sheet.
enum ActivityTab { all, active, completed, cancelled, failed }

class ActivityFilterResult {
  const ActivityFilterResult({
    required this.tab,
    this.dateRange = 'Last 7 days',
    this.location = 'All locations',
  });
  final ActivityTab tab;
  final String dateRange;
  final String location;
}

Future<ActivityFilterResult?> showActivityFiltersSheet(BuildContext context) {
  return showModalBottomSheet<ActivityFilterResult>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    backgroundColor: Colors.transparent,
    barrierColor: Colors.black.withValues(alpha: 0.72),
    builder: (_) => const _ActivityFiltersSheet(),
  );
}

class _ActivityFiltersSheet extends StatefulWidget {
  const _ActivityFiltersSheet();

  @override
  State<_ActivityFiltersSheet> createState() => _ActivityFiltersSheetState();
}

class _ActivityFiltersSheetState extends State<_ActivityFiltersSheet> {
  ActivityTab _status = ActivityTab.all;
  String _dateRange = 'Last 7 days';
  String _location = 'All locations';

  static const _statuses = {
    ActivityTab.all: ('All activity', 'Show everything'),
    ActivityTab.active: ('Active', 'In progress or ongoing'),
    ActivityTab.completed: ('Completed', 'Delivered successfully'),
    ActivityTab.cancelled: ('Cancelled', 'Cancelled by sender or receiver'),
    ActivityTab.failed: ('Failed', 'Delivery attempt failed'),
  };

  static const _dateOptions = [
    'Today',
    'Last 7 days',
    'Last 30 days',
    'This year',
    'All time',
  ];

  static const _locationOptions = [
    'All locations',
    'Ikoyi',
    'Lekki',
    'Victoria Island',
    'Yaba',
    'Surulere',
    'Ikeja',
  ];

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final viewInsets = MediaQuery.of(context).viewInsets.bottom;

    return Padding(
      padding: EdgeInsets.only(bottom: viewInsets),
      child: Container(
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: const BorderRadius.vertical(top: Radius.circular(28)),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.22),
              blurRadius: 40,
              spreadRadius: -8,
              offset: const Offset(0, -6),
            ),
          ],
        ),
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 12, 24, 20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Center(
                child: Container(
                  width: 40,
                  height: 4,
                  decoration: BoxDecoration(
                    color: colors.navy20,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
              const SizedBox(height: 20),

              Row(
                children: [
                  Expanded(
                    child: Text(
                      'Filter activity',
                      style: TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: colors.text,
                        letterSpacing: -0.5,
                        height: 1.15,
                      ),
                    ),
                  ),
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
                    behavior: HitTestBehavior.opaque,
                    child: Padding(
                      padding: const EdgeInsets.all(4),
                      child: Icon(
                        Icons.close_rounded,
                        size: 22,
                        color: colors.textMuted,
                      ),
                    ),
                  ),
                ],
              ),

              const SizedBox(height: 22),

              _SectionLabel('Status'),
              const SizedBox(height: 10),
              for (final entry in _statuses.entries) ...[
                _StatusRadio(
                  title: entry.value.$1,
                  subtitle: entry.value.$2,
                  selected: _status == entry.key,
                  onTap: () => setState(() => _status = entry.key),
                ),
                const SizedBox(height: 8),
              ],

              const SizedBox(height: 18),

              _SectionLabel('Date range'),
              const SizedBox(height: 10),
              _DropdownField(
                value: _dateRange,
                options: _dateOptions,
                icon: Icons.calendar_today_outlined,
                onChanged: (v) => setState(() => _dateRange = v),
              ),

              const SizedBox(height: 18),

              _SectionLabel('Location'),
              const SizedBox(height: 10),
              _DropdownField(
                value: _location,
                options: _locationOptions,
                icon: Icons.location_on_outlined,
                onChanged: (v) => setState(() => _location = v),
              ),

              const SizedBox(height: 24),

              SizedBox(
                height: 54,
                child: FilledButton(
                  onPressed: () {
                    Navigator.of(context).pop(
                      ActivityFilterResult(
                        tab: _status,
                        dateRange: _dateRange,
                        location: _location,
                      ),
                    );
                  },
                  style: FilledButton.styleFrom(
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(14),
                    ),
                    textStyle: const TextStyle(
                      fontSize: 15.5,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Apply filters'),
                ),
              ),
              const SizedBox(height: 8),
              SizedBox(
                height: 44,
                child: TextButton(
                  onPressed: () => setState(() {
                    _status = ActivityTab.all;
                    _dateRange = 'Last 7 days';
                    _location = 'All locations';
                  }),
                  style: TextButton.styleFrom(
                    foregroundColor: colors.textMuted,
                    textStyle: const TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  child: const Text('Reset filters'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ── Small widgets ───────────────────────────────────────────────────

class _SectionLabel extends StatelessWidget {
  const _SectionLabel(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Text(
      text,
      style: TextStyle(
        fontSize: 13,
        fontWeight: FontWeight.w700,
        color: colors.text,
        letterSpacing: -0.1,
      ),
    );
  }
}

class _StatusRadio extends StatelessWidget {
  const _StatusRadio({
    required this.title,
    required this.subtitle,
    required this.selected,
    required this.onTap,
  });

  final String title;
  final String subtitle;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 140),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        decoration: BoxDecoration(
          color: selected ? colors.teal10 : colors.navy10,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: selected
                ? colors.brandTeal500.withValues(alpha: 0.5)
                : colors.divider,
            width: selected ? 1.4 : 1,
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 20,
              height: 20,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color: selected ? colors.brandTeal500 : colors.navy40,
                  width: 2,
                ),
              ),
              child: selected
                  ? Container(
                      width: 10,
                      height: 10,
                      decoration: BoxDecoration(
                        color: colors.brandTeal500,
                        shape: BoxShape.circle,
                      ),
                    )
                  : null,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontSize: 14.5,
                      fontWeight: FontWeight.w700,
                      color: colors.text,
                      letterSpacing: -0.1,
                    ),
                  ),
                  const SizedBox(height: 1),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12,
                      color: colors.textMuted,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DropdownField extends StatelessWidget {
  const _DropdownField({
    required this.value,
    required this.options,
    required this.icon,
    required this.onChanged,
  });

  final String value;
  final List<String> options;
  final IconData icon;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Container(
      height: 52,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: colors.divider),
      ),
      child: Row(
        children: [
          Icon(icon, size: 18, color: colors.textMuted),
          const SizedBox(width: 12),
          Expanded(
            child: DropdownButtonHideUnderline(
              child: DropdownButton<String>(
                value: value,
                isExpanded: true,
                icon: Icon(
                  Icons.keyboard_arrow_down_rounded,
                  color: colors.text,
                ),
                dropdownColor: colors.surface,
                style: TextStyle(
                  fontSize: 14.5,
                  fontWeight: FontWeight.w600,
                  color: colors.text,
                ),
                items: options
                    .map(
                      (o) => DropdownMenuItem<String>(
                        value: o,
                        child: Text(o),
                      ),
                    )
                    .toList(),
                onChanged: (v) {
                  if (v != null) onChanged(v);
                },
              ),
            ),
          ),
        ],
      ),
    );
  }
}