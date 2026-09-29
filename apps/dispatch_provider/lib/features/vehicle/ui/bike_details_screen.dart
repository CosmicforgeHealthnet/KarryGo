import 'package:flutter/material.dart';

import '../../../core/app_mode.dart';
import '../../../core/theme/theme_extensions.dart';
import '../state/vehicle_controller.dart';
import 'bike_documents_screen.dart';
import 'bike_eligibility_screen.dart';
import 'edit_bike_screen.dart';

class BikeDetailsScreen extends StatefulWidget {
  const BikeDetailsScreen({
    super.key,
    required this.vehicleController,
    required this.vehicleId,
  });

  final VehicleController vehicleController;
  final String vehicleId;

  @override
  State<BikeDetailsScreen> createState() => _BikeDetailsScreenState();
}

class _BikeDetailsScreenState extends State<BikeDetailsScreen> {
  bool _isLoading = true;
  String? _loadError;
  Map<String, dynamic>? _bike;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result =
        await widget.vehicleController.getVehicle(widget.vehicleId);
    if (!mounted) return;
    result.when(
      success: (bike) => setState(() {
        _bike = bike;
        _isLoading = false;
      }),
      failure: (error) => setState(() {
        _loadError = error.message;
        _isLoading = false;
      }),
    );
  }

  Future<void> _openEdit() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => EditBikeScreen(
          vehicleController: widget.vehicleController,
          bike: _bike!,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) _load();
  }

  Future<void> _openDocuments() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => BikeDocumentsScreen(
          vehicleController: widget.vehicleController,
          vehicleId: widget.vehicleId,
        ),
      ),
    );
    if (mounted) _load();
  }

  /// Opens the (mock-only) eligibility preview screen. Hidden in LIVE because
  /// the backend has no eligibility endpoint for vehicles yet.
  Future<void> _openEligibility() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => BikeEligibilityScreen(
          vehicleController: widget.vehicleController,
          vehicleId: widget.vehicleId,
        ),
      ),
    );
  }

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
              child: _isLoading
                  ? Center(
                      child: CircularProgressIndicator(
                        color: c.brandTeal500,
                      ),
                    )
                  : _loadError != null
                      ? Center(
                          child: Padding(
                            padding: const EdgeInsets.all(32),
                            child: Text(
                              _loadError!,
                              style: TextStyle(
                                fontSize: 15,
                                color: c.textMuted,
                              ),
                              textAlign: TextAlign.center,
                            ),
                          ),
                        )
                      : _bike == null
                          ? const SizedBox.shrink()
                          : _buildBody(c, _bike!),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildBody(dynamic c, Map<String, dynamic> bike) {
    // Backend field is verification_status, values are:
    // unverified | pending | verified | rejected | suspended.
    final status =
        (bike['verification_status'] as String? ?? 'unverified').toLowerCase();

    final (label, fg, bg) = switch (status) {
      'verified' => ('Active', c.success, c.successSoft),
      'pending' => ('Pending', c.warning, c.warningSoft),
      'rejected' => ('Rejected', c.danger, c.dangerSoft),
      'suspended' => ('Suspended', c.danger, c.dangerSoft),
      _ => ('Unverified', c.textMuted, c.navy10),
    };

    final brand = (bike['brand'] as String? ?? '').trim();
    final model = (bike['model'] as String? ?? '').trim();
    final name = [brand, model].where((s) => s.isNotEmpty).join(' ');

    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
      children: [
        // ── Hero card ──
        Container(
          padding: const EdgeInsets.all(20),
          decoration: BoxDecoration(
            color: c.surface,
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: c.divider),
          ),
          child: Column(
            children: [
              Container(
                width: 140,
                height: 140,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: c.teal10,
                  borderRadius: BorderRadius.circular(20),
                ),
                child: Icon(
                  Icons.two_wheeler_rounded,
                  size: 76,
                  color: c.brandTeal600,
                ),
              ),
              const SizedBox(height: 20),
              Text(
                name.isEmpty ? '—' : name,
                style: TextStyle(
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.3,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                bike['plate_number'] as String? ?? '—',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                  letterSpacing: 0.3,
                ),
              ),
              const SizedBox(height: 12),
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 5,
                ),
                decoration: BoxDecoration(
                  color: bg,
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  label,
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    color: fg,
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 20),

        // ── Details ──
        Container(
          padding: const EdgeInsets.all(18),
          decoration: BoxDecoration(
            color: c.surface,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: c.divider),
          ),
          child: Column(
            children: [
              Row(
                children: [
                  Expanded(
                    child: _KV(
                      label: 'Make',
                      value: bike['brand'] as String? ?? '—',
                    ),
                  ),
                  Expanded(
                    child: _KV(
                      label: 'Model',
                      value: bike['model'] as String? ?? '—',
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 18),
              Row(
                children: [
                  Expanded(
                    child: _KV(
                      label: 'Year',
                      value: '${bike['year'] ?? '—'}',
                    ),
                  ),
                  Expanded(
                    child: _KV(
                      label: 'Color',
                      value: bike['color'] as String? ?? '—',
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
        const SizedBox(height: 20),

        // ── Eligibility row (mock-only) ──
        // Hidden in LIVE until the backend exposes a real eligibility
        // endpoint for vehicles. The screen is a Figma-accurate placeholder
        // and would lie to real riders.
        if (AppModeConfig.instance.isMock) ...[
          GestureDetector(
            onTap: _openEligibility,
            behavior: HitTestBehavior.opaque,
            child: Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: c.surface,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: c.divider),
              ),
              child: Row(
                children: [
                  Icon(
                    Icons.verified_outlined,
                    size: 22,
                    color: c.brandTeal600,
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Text(
                      'Eligibility status',
                      style: TextStyle(
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                      ),
                    ),
                  ),
                  Icon(
                    Icons.chevron_right_rounded,
                    size: 22,
                    color: c.textMuted,
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 12),
        ],

        // ── Documents row ──
        GestureDetector(
          onTap: _openDocuments,
          behavior: HitTestBehavior.opaque,
          child: Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: c.surface,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: c.divider),
            ),
            child: Row(
              children: [
                Icon(
                  Icons.folder_outlined,
                  size: 22,
                  color: c.text,
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Text(
                    'Bike documents',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w700,
                      color: c.text,
                    ),
                  ),
                ),
                Icon(
                  Icons.chevron_right_rounded,
                  size: 22,
                  color: c.textMuted,
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 24),

        // ── Edit bike ──
        SizedBox(
          height: 52,
          width: double.infinity,
          child: FilledButton(
            onPressed: _openEdit,
            style: FilledButton.styleFrom(
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: const Text('Edit bike'),
          ),
        ),
      ],
    );
  }
}

class _KV extends StatelessWidget {
  const _KV({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 12.5,
            fontWeight: FontWeight.w500,
            color: c.textMuted,
            letterSpacing: 0.2,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: c.text,
            letterSpacing: -0.2,
          ),
        ),
      ],
    );
  }
}