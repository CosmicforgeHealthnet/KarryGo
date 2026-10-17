import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../state/vehicle_controller.dart';
import 'upload_vehicle_document_screen.dart';

class BikeDocumentsScreen extends StatefulWidget {
  const BikeDocumentsScreen({
    super.key,
    required this.vehicleController,
    required this.vehicleId,
  });

  final VehicleController vehicleController;
  final String vehicleId;

  @override
  State<BikeDocumentsScreen> createState() => _BikeDocumentsScreenState();
}

class _BikeDocumentsScreenState extends State<BikeDocumentsScreen> {
  bool _isLoading = true;
  String? _loadError;
  List<Map<String, dynamic>> _docs = [];

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
    final result = await widget.vehicleController.listVehicleDocuments(
      widget.vehicleId,
    );
    if (!mounted) return;
    result.when(
      success: (list) => setState(() {
        _docs = list;
        _isLoading = false;
      }),
      failure: (error) => setState(() {
        _loadError = error.message;
        _isLoading = false;
      }),
    );
  }

  Future<void> _openUpload({String? documentType}) async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => UploadVehicleDocumentScreen(
          vehicleController: widget.vehicleController,
          vehicleId: widget.vehicleId,
          initialDocumentType: documentType,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) _load();
  }

  @override
  Widget build(BuildContext context) => AccountPage(
    title: 'Vehicle documents',
    symbol: JourneySymbol.review,
    description: 'Upload your documents and review their recorded status.',
    onRefresh: _load,
    bottom: FilledButton.icon(
      onPressed: _isLoading ? null : () => _openUpload(),
      icon: const Icon(Icons.add_rounded),
      label: const Text('Upload document'),
    ),
    children: [
      if (_isLoading)
        const SizedBox(height: 220, child: ContentSkeleton(rows: 2))
      else if (_loadError != null)
        ActionNotice(message: _loadError!, isError: true, onRetry: _load)
      else ...[
        if (_docs.isEmpty)
          const Padding(
            padding: EdgeInsets.only(bottom: 24),
            child: Text('No documents uploaded yet.'),
          ),
        for (final doc in _docs)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: DeliveryPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  AccountMenuTile(
                    title:
                        doc['document_type']?.toString().replaceAll('_', ' ') ??
                        'Document',
                    subtitle:
                        doc['verification_status']?.toString() ??
                        doc['status']?.toString() ??
                        'Uploaded',
                    symbol: JourneySymbol.review,
                  ),
                  if (doc['uploaded_at'] != null)
                    AccountValue(
                      label: 'Uploaded',
                      value: doc['uploaded_at'].toString(),
                    ),
                  if (doc['expiry_date'] != null)
                    AccountValue(
                      label: 'Expires',
                      value: doc['expiry_date'].toString(),
                    ),
                  if (doc['rejection_reason'] != null)
                    AccountValue(
                      label: 'Review feedback',
                      value: doc['rejection_reason'].toString(),
                    ),
                  OutlinedButton(
                    onPressed: () => _openUpload(
                      documentType: doc['document_type']?.toString(),
                    ),
                    child: const Text('Replace document'),
                  ),
                ],
              ),
            ),
          ),
      ],
    ],
  );
}
