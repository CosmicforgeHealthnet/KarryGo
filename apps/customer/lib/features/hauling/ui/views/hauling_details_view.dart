import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../models/hauling_models.dart';
import '../../state/hauling_booking_controller.dart';
import '../widgets/hauling_flow_helpers.dart';

class HaulingDetailsView extends StatefulWidget {
  const HaulingDetailsView({super.key, required this.controller, required this.state});

  final HaulingBookingController controller;
  final HaulingBookingState state;

  @override
  State<HaulingDetailsView> createState() => _HaulingDetailsViewState();
}

class _HaulingDetailsViewState extends State<HaulingDetailsView> {
  final _descCtrl = TextEditingController();
  bool _wantsSchedule = false;

  HaulingBookingController get _ctrl => widget.controller;
  HaulingBookingState get _state => widget.state;

  @override
  void initState() {
    super.initState();
    _descCtrl.text = _state.cargoDescription;
    _wantsSchedule = _state.scheduledAt != null;
  }

  @override
  void dispose() {
    _descCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return haulingFlowScaffold(
      title: 'Truck Hauling',
      onBack: _ctrl.backToTierSelection,
      body: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text(
              'Providing the right information helps to improve processes, matching with the right truck and your overall experience.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 13, height: 1.5),
            ),
            const SizedBox(height: 20),

            // ── What are you moving? ──────────────────────────────────────
            haulingSectionLabel('What are you moving?'),
            TextField(
              controller: _descCtrl,
              onChanged: (v) {
                setState(() {});
                _ctrl.setCargoDescription(v);
              },
              decoration: _inputDecoration('e.g. Furniture, Equipment, Sand...'),
            ),
            const SizedBox(height: 20),

            // ── Weight category ───────────────────────────────────────────
            haulingSectionLabel('Load weight category'),
            const SizedBox(height: 2),
            const Text(
              'Let us know how heavy the item is.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            ),
            const SizedBox(height: 8),
            ...WeightCategory.values.map((cat) => _CheckRow(
              label: cat.displayLabel,
              checked: _state.weightCategory == cat,
              onTap: () { setState(() {}); _ctrl.setWeightCategory(cat); },
            )),
            const SizedBox(height: 20),

            // ── Truck type ────────────────────────────────────────────────
            haulingSectionLabel('Truck Type'),
            const SizedBox(height: 8),
            DropdownButtonFormField<HaulingTruckTypeOption>(
              value: _state.truckTypeOption,
              hint: const Text('Select truck type', style: TextStyle(color: CustomerFigmaColors.muted)),
              decoration: InputDecoration(
                filled: true,
                fillColor: Colors.white,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.border),
                ),
                enabledBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.border),
                ),
                focusedBorder: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(10),
                  borderSide: const BorderSide(color: CustomerFigmaColors.primary),
                ),
                contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
              ),
              items: HaulingTruckTypeOption.values
                  .map((opt) => DropdownMenuItem(value: opt, child: Text(opt.displayLabel)))
                  .toList(),
              onChanged: (opt) {
                if (opt != null) { setState(() {}); _ctrl.setTruckTypeOption(opt); }
              },
            ),
            const SizedBox(height: 20),

            // ── Loaders ───────────────────────────────────────────────────
            haulingSectionLabel('Do you need loaders?'),
            const SizedBox(height: 2),
            const Text(
              'Let us know if you need extra hands to help you load the truck.',
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            ),
            const SizedBox(height: 8),
            _CheckRow(
              label: 'No',
              checked: !_state.requiresHelpers,
              onTap: () { setState(() {}); _ctrl.setRequiresHelpers(false); },
            ),
            _CheckRow(
              label: 'Yes',
              checked: _state.requiresHelpers,
              onTap: () { setState(() {}); _ctrl.setRequiresHelpers(true); },
            ),
            if (_state.requiresHelpers) ...[
              const SizedBox(height: 10),
              const Text(
                'How many loaders do you need?',
                style: TextStyle(color: CustomerFigmaColors.text, fontSize: 13, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  _CounterButton(
                    icon: Icons.remove,
                    onTap: _state.helperCount > 1
                        ? () { setState(() {}); _ctrl.setHelperCount(_state.helperCount - 1); }
                        : null,
                  ),
                  Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 20),
                    child: Text(
                      '${_state.helperCount}',
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontWeight: FontWeight.w800,
                        fontSize: 20,
                      ),
                    ),
                  ),
                  _CounterButton(
                    icon: Icons.add,
                    onTap: () { setState(() {}); _ctrl.setHelperCount(_state.helperCount + 1); },
                  ),
                ],
              ),
            ],
            const SizedBox(height: 20),

            // ── Schedule ──────────────────────────────────────────────────
            haulingSectionLabel("Do you want to schedule?"),
            const SizedBox(height: 2),
            const Text(
              "Let's know if you want to schedule ahead.",
              style: TextStyle(color: CustomerFigmaColors.muted, fontSize: 12),
            ),
            const SizedBox(height: 8),
            _CheckRow(
              label: 'No, book now',
              checked: !_wantsSchedule,
              onTap: () {
                setState(() => _wantsSchedule = false);
                _ctrl.setScheduledAt(null);
              },
            ),
            _CheckRow(
              label: 'Yes, schedule a time',
              checked: _wantsSchedule,
              onTap: () => setState(() => _wantsSchedule = true),
            ),
            if (_wantsSchedule) ...[
              const SizedBox(height: 10),
              GestureDetector(
                onTap: () => _pickDateTime(context),
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(color: CustomerFigmaColors.border),
                  ),
                  child: Row(
                    children: [
                      const Icon(Icons.calendar_today_outlined, color: CustomerFigmaColors.primary, size: 18),
                      const SizedBox(width: 10),
                      Text(
                        _state.scheduledAt != null
                            ? _formatDateTime(_state.scheduledAt!)
                            : 'Select date and time',
                        style: TextStyle(
                          color: _state.scheduledAt != null ? CustomerFigmaColors.text : CustomerFigmaColors.muted,
                          fontSize: 14,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
            const SizedBox(height: 8),
          ],
        ),
      ),
      bottom: Column(
        children: [
          if (_state.error != null) ...[
            Text(
              _state.error!,
              style: const TextStyle(color: Colors.red, fontSize: 12),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 8),
          ],
          FigmaPrimaryButton(
            label: 'Continue',
            isLoading: _state.isLoading,
            onPressed: _state.detailsReady ? _ctrl.proceedFromDetailsToPackageInfo : null,
          ),
        ],
      ),
    );
  }

  Future<void> _pickDateTime(BuildContext context) async {
    final now = DateTime.now();
    final date = await showDatePicker(
      context: context,
      initialDate: now.add(const Duration(hours: 2)),
      firstDate: now,
      lastDate: now.add(const Duration(days: 30)),
    );
    if (date == null || !mounted || !context.mounted) return;
    final time = await showTimePicker(context: context, initialTime: TimeOfDay.now());
    if (time == null) return;
    _ctrl.setScheduledAt(DateTime(date.year, date.month, date.day, time.hour, time.minute));
  }

  String _formatDateTime(DateTime dt) {
    const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    final h = dt.hour % 12 == 0 ? 12 : dt.hour % 12;
    final m = dt.minute.toString().padLeft(2, '0');
    final ampm = dt.hour < 12 ? 'AM' : 'PM';
    return '${dt.day} ${months[dt.month - 1]} ${dt.year} · $h:$m $ampm';
  }

  InputDecoration _inputDecoration(String hint) => InputDecoration(
    hintText: hint,
    hintStyle: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
    filled: true,
    fillColor: Colors.white,
    border: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.border),
    ),
    enabledBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.border),
    ),
    focusedBorder: OutlineInputBorder(
      borderRadius: BorderRadius.circular(10),
      borderSide: const BorderSide(color: CustomerFigmaColors.primary),
    ),
    contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
  );
}

// ─── Shared sub-widgets ────────────────────────────────────────────────────────

class _CheckRow extends StatelessWidget {
  const _CheckRow({required this.label, required this.checked, required this.onTap});

  final String label;
  final bool checked;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 5),
        child: Row(
          children: [
            AnimatedContainer(
              duration: const Duration(milliseconds: 150),
              width: 20, height: 20,
              decoration: BoxDecoration(
                color: checked ? CustomerFigmaColors.primary : Colors.white,
                borderRadius: BorderRadius.circular(4),
                border: Border.all(
                  color: checked ? CustomerFigmaColors.primary : CustomerFigmaColors.border,
                  width: 1.5,
                ),
              ),
              child: checked ? const Icon(Icons.check, color: Colors.white, size: 14) : null,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  color: checked ? CustomerFigmaColors.primary : CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: checked ? FontWeight.w600 : FontWeight.w400,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CounterButton extends StatelessWidget {
  const _CounterButton({required this.icon, this.onTap});

  final IconData icon;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: 36, height: 36,
        decoration: BoxDecoration(
          color: onTap != null ? CustomerFigmaColors.primaryTint : Colors.grey[100],
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: onTap != null ? CustomerFigmaColors.primary : Colors.grey[300]!,
          ),
        ),
        child: Icon(
          icon,
          color: onTap != null ? CustomerFigmaColors.primary : Colors.grey[400],
          size: 18,
        ),
      ),
    );
  }
}
