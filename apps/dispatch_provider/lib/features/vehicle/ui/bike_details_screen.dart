import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/journey_icon.dart';
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
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result = await widget.vehicleController.getVehicle(widget.vehicleId);
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
    final bike = _bike;
    final status = bike?['verification_status']?.toString() ?? 'unverified';
    return AccountPage(
      title: 'Vehicle details',
      symbol: JourneySymbol.fleet,
      onRefresh: _load,
      children: [
        if (_isLoading)
          const SizedBox(height: 220, child: ContentSkeleton(rows: 2))
        else if (_loadError != null)
          ActionNotice(message: _loadError!, isError: true, onRetry: _load)
        else if (bike != null) ...[
          DeliveryPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  status == 'pending'
                      ? 'Under review'
                      : status.replaceAll('_', ' '),
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    color: status == 'verified'
                        ? context.palette.success
                        : ['rejected', 'suspended'].contains(status)
                        ? context.palette.danger
                        : context.palette.warning,
                  ),
                ),
                const SizedBox(height: 12),
                for (final field in const {
                  'brand': 'Make',
                  'model': 'Model',
                  'year': 'Year',
                  'color': 'Colour',
                  'plate_number': 'Plate number',
                }.entries)
                  AccountValue(
                    label: field.value,
                    value: bike[field.key]?.toString() ?? '',
                  ),
              ],
            ),
          ),
          const SizedBox(height: 24),
          AccountSection(
            title: 'Manage vehicle',
            children: [
              AccountMenuTile(
                title: 'Edit vehicle details',
                symbol: JourneySymbol.preferences,
                onTap: _openEdit,
              ),
              AccountMenuTile(
                title: 'Documents',
                subtitle: 'Registration and insurance',
                symbol: JourneySymbol.review,
                onTap: _openDocuments,
              ),
              if (AppModeConfig.instance.isMock)
                AccountMenuTile(
                  title: 'Demo eligibility preview',
                  symbol: JourneySymbol.shield,
                  onTap: _openEligibility,
                ),
            ],
          ),
        ],
      ],
    );
  }
}
