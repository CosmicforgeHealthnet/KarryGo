import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
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
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result =
        await widget.vehicleController.listVehicleDocuments(widget.vehicleId);
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
                      : ListView(
                          padding:
                              const EdgeInsets.fromLTRB(20, 8, 20, 24),
                          children: [
                            Text(
                              'Bike documents',
                              style: TextStyle(
                                fontSize: 26,
                                fontWeight: FontWeight.w800,
                                color: c.text,
                                letterSpacing: -0.4,
                              ),
                            ),
                            const SizedBox(height: 6),
                            Text(
                              'Keep your documents valid and up to date.',
                              style: TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w500,
                                color: c.textMuted,
                                height: 1.45,
                              ),
                            ),
                            const SizedBox(height: 28),

                            if (_docs.isEmpty)
                              Container(
                                padding: const EdgeInsets.all(24),
                                decoration: BoxDecoration(
                                  color: c.surface,
                                  borderRadius: BorderRadius.circular(16),
                                  border: Border.all(color: c.divider),
                                ),
                                child: Column(
                                  children: [
                                    Icon(
                                      Icons.folder_open_outlined,
                                      size: 36,
                                      color: c.navy40,
                                    ),
                                    const SizedBox(height: 10),
                                    Text(
                                      'No documents yet',
                                      style: TextStyle(
                                        fontSize: 15,
                                        fontWeight: FontWeight.w700,
                                        color: c.text,
                                      ),
                                    ),
                                    const SizedBox(height: 4),
                                    Text(
                                      'Upload your registration and insurance certificate.',
                                      textAlign: TextAlign.center,
                                      style: TextStyle(
                                        fontSize: 13,
                                        color: c.textMuted,
                                        height: 1.4,
                                      ),
                                    ),
                                  ],
                                ),
                              )
                            else
                              for (final doc in _docs) ...[
                                _DocRow(
                                  doc: doc,
                                  onTap: () => _openUpload(
                                    documentType:
                                        doc['document_type'] as String?,
                                  ),
                                ),
                                const SizedBox(height: 12),
                              ],
                          ],
                        ),
            ),

            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: () => _openUpload(),
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Manage documents'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Document row — reads real backend fields
//
// Backend BikeDocument shape:
//   {id, bike_id, provider_id, document_type, file_url,
//    file_size, mime_type, expiry_date, uploaded_at}
//
// There is no per-document status; a document exists or it doesn't.
// ═══════════════════════════════════════════════════════════════════════════

class _DocRow extends StatelessWidget {
  const _DocRow({required this.doc, required this.onTap});
  final Map<String, dynamic> doc;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    final docType = (doc['document_type'] as String? ?? '').toLowerCase();
    final label = switch (docType) {
      'registration' => 'Registration Certificate',
      'insurance' => 'Insurance Certificate',
      _ => docType.isEmpty ? 'Document' : docType,
    };

    final uploadedAt = doc['uploaded_at'] as String?;
    final expiry = doc['expiry_date'] as String?;

    final subtitleParts = <String>[];
    if (uploadedAt != null) subtitleParts.add('Uploaded $uploadedAt');
    if (expiry != null) subtitleParts.add('Expires $expiry');

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: c.surface,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: c.divider),
        ),
        child: Row(
          children: [
            Icon(
              Icons.check_circle_rounded,
              size: 22,
              color: c.success,
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: c.text,
                    ),
                  ),
                  if (subtitleParts.isNotEmpty) ...[
                    const SizedBox(height: 3),
                    Text(
                      subtitleParts.join(' · '),
                      style: TextStyle(
                        fontSize: 11.5,
                        color: c.textMuted,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            Text(
              'Replace',
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w700,
                color: c.brandTeal600,
              ),
            ),
          ],
        ),
      ),
    );
  }
}