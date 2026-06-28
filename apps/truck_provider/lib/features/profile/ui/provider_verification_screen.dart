import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../home/ui/widgets/provider_app_colors.dart';
import '../state/provider_profile_controller.dart';
import 'provider_face_verification_screen.dart';
import 'widgets/provider_profile_widgets.dart';

/// Verification & Documents (Figma 2133–2141). Captures the driver's licence
/// number + expiry, uploads the three required documents, and submits the
/// profile for verification.
class ProviderVerificationScreen extends StatefulWidget {
  const ProviderVerificationScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProviderVerificationScreen> createState() => _ProviderVerificationScreenState();
}

class _ProviderVerificationScreenState extends State<ProviderVerificationScreen> {
  late final TextEditingController _licenseCtrl;
  String? _expiryYear;
  String? _expiryDate;
  bool _submitting = false;

  late final _DocSlot _govId;
  late final _DocSlot _license;
  late final _DocSlot _vehicleReg;

  @override
  void initState() {
    super.initState();
    final p = widget.profileController.profile;
    _licenseCtrl = TextEditingController(text: p?.driverLicenseNumber ?? '');
    _expiryYear = (p?.licenseExpiryYear.isNotEmpty ?? false) ? p!.licenseExpiryYear : null;
    _expiryDate = (p?.licenseExpiryDate.isNotEmpty ?? false) ? p!.licenseExpiryDate : null;
    _govId = _DocSlot(url: p?.govIdUrl ?? '');
    _license = _DocSlot(url: p?.driverLicenseUrl ?? '');
    _vehicleReg = _DocSlot(url: p?.vehicleRegUrl ?? '');
    _licenseCtrl.addListener(() => setState(() {}));
  }

  @override
  void dispose() {
    _licenseCtrl.dispose();
    super.dispose();
  }

  bool get _canSubmit =>
      !_submitting &&
      _licenseCtrl.text.trim().isNotEmpty &&
      _govId.hasFile &&
      _license.hasFile &&
      _vehicleReg.hasFile;

  Future<void> _pick(_DocSlot slot) async {
    final file = await ImagePicker().pickImage(source: ImageSource.gallery, imageQuality: 85);
    if (file == null || !mounted) return;
    setState(() => slot.uploading = true);
    try {
      final url = await widget.profileController.uploadDocument(file);
      if (!mounted) return;
      setState(() {
        slot.url = url ?? '';
        slot.fileName = file.name;
        slot.uploading = false;
      });
    } on ApiException catch (e) {
      if (mounted) {
        setState(() => slot.uploading = false);
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    } catch (_) {
      if (mounted) {
        setState(() => slot.uploading = false);
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Could not upload the document. Please try again.')),
        );
      }
    }
  }

  Future<void> _pickExpiryDate() async {
    final now = DateTime.now();
    final picked = await showDatePicker(
      context: context,
      initialDate: now,
      firstDate: now,
      lastDate: DateTime(now.year + 20),
    );
    if (picked != null && mounted) {
      setState(() => _expiryDate =
          '${picked.year}-${picked.month.toString().padLeft(2, '0')}-${picked.day.toString().padLeft(2, '0')}');
    }
  }

  Future<void> _submit() async {
    setState(() => _submitting = true);
    final ok = await widget.profileController.saveVerification(
      licenseNumber: _licenseCtrl.text.trim(),
      expiryYear: _expiryYear ?? '',
      expiryDate: _expiryDate ?? '',
      govIdUrl: _govId.url,
      driverLicenseUrl: _license.url,
      vehicleRegUrl: _vehicleReg.url,
    );
    if (!mounted) return;
    setState(() => _submitting = false);
    if (!ok) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(widget.profileController.error ?? 'Could not submit verification.')),
      );
      return;
    }
    // Visual KYC face step, then back to profile.
    await Navigator.of(context).push(
      MaterialPageRoute(builder: (_) => const ProviderFaceVerificationScreen()),
    );
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final provider = widget.profileController.profile;
    final status = provider?.verificationLabel ?? 'Unverified';
    final statusColor = provider?.isVerified == true
        ? kProviderGreen
        : (provider?.isProcessing == true ? kProviderAmber : kProviderRejectText);
    final year = DateTime.now().year;
    final years = [for (var y = year; y <= year + 15; y++) y.toString()];

    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
              child: ProviderProfileHeader(title: 'Verification & Documents', subtitle: 'Verify your identity'),
            ),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
                children: [
                  Row(
                    children: [
                      const Text('Verification Status:  ',
                          style: TextStyle(color: kProviderText, fontSize: 16, fontWeight: FontWeight.w800)),
                      Text(status,
                          style: TextStyle(color: statusColor, fontSize: 16, fontWeight: FontWeight.w800)),
                    ],
                  ),
                  const SizedBox(height: 18),

                  const ProviderFieldLabel("Your Driver's license no"),
                  TextField(controller: _licenseCtrl, decoration: providerWhiteField('1234567890')),
                  const SizedBox(height: 16),

                  Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const ProviderFieldLabel('Expiry Year'),
                            DropdownButtonFormField<String>(
                              initialValue: _expiryYear,
                              isExpanded: true,
                              hint: const Text('Select Year', style: TextStyle(color: kProviderMuted, fontSize: 14)),
                              icon: const Icon(Icons.keyboard_arrow_down_rounded, color: kProviderMuted),
                              decoration: providerWhiteField(''),
                              items: years.map((y) => DropdownMenuItem(value: y, child: Text(y))).toList(),
                              onChanged: (v) => setState(() => _expiryYear = v),
                            ),
                          ],
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const ProviderFieldLabel('Expiry Date'),
                            GestureDetector(
                              onTap: _pickExpiryDate,
                              child: AbsorbPointer(
                                child: TextField(
                                  controller: TextEditingController(text: _expiryDate ?? ''),
                                  decoration: providerWhiteField(
                                    'Select Expiry Date',
                                    suffixIcon: const Icon(Icons.calendar_today_rounded, size: 18, color: kProviderMuted),
                                  ),
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 20),

                  _UploadField(
                    title: 'Upload Approved Government ID',
                    helper: 'We need you to provide a photo of your Government ID to enable us to verify your identity for user safety and security.',
                    slot: _govId,
                    onTap: () => _pick(_govId),
                    onRemove: () => setState(() => _govId.reset()),
                  ),
                  const SizedBox(height: 20),
                  _UploadField(
                    title: "Upload Driver's License",
                    helper: "We need you to provide a photo of your driver's license to enable us to verify your identity for user safety and security.",
                    slot: _license,
                    onTap: () => _pick(_license),
                    onRemove: () => setState(() => _license.reset()),
                  ),
                  const SizedBox(height: 20),
                  _UploadField(
                    title: 'Upload Vehicle Registration',
                    helper: 'We need you to provide a photo of your Vehicle Registration to enable us to verify your identity for user safety and security.',
                    slot: _vehicleReg,
                    onTap: () => _pick(_vehicleReg),
                    onRemove: () => setState(() => _vehicleReg.reset()),
                  ),
                ],
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: ProviderPrimaryButton(
                label: 'Next',
                isLoading: _submitting,
                onPressed: _canSubmit ? _submit : null,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _DocSlot {
  _DocSlot({this.url = ''});
  String url;
  String fileName = '';
  bool uploading = false;

  bool get hasFile => url.isNotEmpty;
  void reset() {
    url = '';
    fileName = '';
    uploading = false;
  }
}

class _UploadField extends StatelessWidget {
  const _UploadField({
    required this.title,
    required this.helper,
    required this.slot,
    required this.onTap,
    required this.onRemove,
  });

  final String title;
  final String helper;
  final _DocSlot slot;
  final VoidCallback onTap;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title, style: const TextStyle(color: kProviderText, fontSize: 15, fontWeight: FontWeight.w800)),
        const SizedBox(height: 6),
        Text(helper, style: const TextStyle(color: kProviderGreen, fontSize: 12, height: 1.5)),
        const SizedBox(height: 10),
        if (slot.uploading)
          _uploadingRow()
        else if (slot.hasFile)
          _doneRow(context)
        else
          _emptyBox(),
      ],
    );
  }

  Widget _emptyBox() {
    return GestureDetector(
      onTap: onTap,
      child: DottedBorderBox(
        child: Column(
          children: [
            const Icon(Icons.file_download_outlined, color: kProviderText, size: 28),
            const SizedBox(height: 8),
            const Text('Upload ID', style: TextStyle(color: kProviderText, fontSize: 14, fontWeight: FontWeight.w700)),
            const SizedBox(height: 6),
            const Text(
              "(International passport, NIN, Voter's card or Driver's License)",
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 10),
            ),
            const SizedBox(height: 2),
            const Text('Supported File type: JPEG, PNG', style: TextStyle(color: kProviderMuted, fontSize: 10)),
            const Text('Maximum File Size: 500MB', style: TextStyle(color: kProviderMuted, fontSize: 10)),
          ],
        ),
      ),
    );
  }

  Widget _uploadingRow() {
    return Row(
      children: [
        const Expanded(
          child: ClipRRect(
            borderRadius: BorderRadius.all(Radius.circular(8)),
            child: LinearProgressIndicator(
              minHeight: 8,
              backgroundColor: kProviderBorder,
              valueColor: AlwaysStoppedAnimation(kProviderGreen),
            ),
          ),
        ),
        const SizedBox(width: 12),
        const SizedBox.square(
          dimension: 22,
          child: CircularProgressIndicator(strokeWidth: 2.4, color: kProviderGreen),
        ),
      ],
    );
  }

  Widget _doneRow(BuildContext context) {
    final isNetwork = slot.url.startsWith('http');
    return Row(
      children: [
        // Thumbnail of the uploaded document (tap to preview full-screen). Falls
        // back to the generic icon if the URL is not a network image or fails.
        GestureDetector(
          onTap: isNetwork ? () => _previewDocument(context, slot.url) : null,
          child: ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: SizedBox(
              width: 56,
              height: 56,
              child: isNetwork
                  ? Image.network(
                      slot.url,
                      fit: BoxFit.cover,
                      loadingBuilder: (context, child, progress) =>
                          progress == null ? child : _thumbPlaceholder(loading: true),
                      errorBuilder: (_, _, _) => _thumbPlaceholder(),
                    )
                  : _thumbPlaceholder(),
            ),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Row(
                children: [
                  Icon(Icons.check_circle_rounded, color: kProviderGreen, size: 16),
                  SizedBox(width: 4),
                  Text('Uploaded', style: TextStyle(color: kProviderGreen, fontSize: 13, fontWeight: FontWeight.w700)),
                ],
              ),
              if (slot.fileName.isNotEmpty) ...[
                const SizedBox(height: 2),
                Text(
                  slot.fileName,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(color: kProviderMuted, fontSize: 12),
                ),
              ] else if (isNetwork) ...[
                const SizedBox(height: 2),
                const Text('Tap to view', style: TextStyle(color: kProviderMuted, fontSize: 12)),
              ],
            ],
          ),
        ),
        GestureDetector(
          onTap: onRemove,
          child: Container(
            width: 26,
            height: 26,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(color: kProviderBorder),
            ),
            child: const Icon(Icons.close_rounded, size: 16, color: kProviderMuted),
          ),
        ),
      ],
    );
  }

  Widget _thumbPlaceholder({bool loading = false}) {
    return Container(
      color: kProviderGreenTint,
      child: Center(
        child: loading
            ? const SizedBox.square(
                dimension: 18,
                child: CircularProgressIndicator(strokeWidth: 2, color: kProviderGreen),
              )
            : const Icon(Icons.image_rounded, color: kProviderGreen, size: 20),
      ),
    );
  }

  void _previewDocument(BuildContext context, String url) {
    Navigator.of(context).push(
      MaterialPageRoute(
        fullscreenDialog: true,
        builder: (_) => _DocumentPreviewScreen(url: url),
      ),
    );
  }
}

/// Full-screen, zoomable preview of an uploaded document image.
class _DocumentPreviewScreen extends StatelessWidget {
  const _DocumentPreviewScreen({required this.url});
  final String url;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        foregroundColor: Colors.white,
        elevation: 0,
      ),
      body: Center(
        child: InteractiveViewer(
          minScale: 0.5,
          maxScale: 4,
          child: Image.network(
            url,
            fit: BoxFit.contain,
            loadingBuilder: (context, child, progress) => progress == null
                ? child
                : const CircularProgressIndicator(color: Colors.white),
            errorBuilder: (_, _, _) => const Icon(Icons.broken_image_rounded, color: Colors.white54, size: 64),
          ),
        ),
      ),
    );
  }
}

/// Dashed-border container used for the empty upload state.
class DottedBorderBox extends StatelessWidget {
  const DottedBorderBox({super.key, required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _DashedRectPainter(),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 22),
        child: child,
      ),
    );
  }
}

class _DashedRectPainter extends CustomPainter {
  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = kProviderBorder
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1.4;
    const radius = 12.0;
    final rrect = RRect.fromRectAndRadius(
      Offset.zero & size,
      const Radius.circular(radius),
    );
    final path = Path()..addRRect(rrect);
    const dash = 6.0;
    const gap = 5.0;
    for (final metric in path.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        canvas.drawPath(metric.extractPath(distance, distance + dash), paint);
        distance += dash + gap;
      }
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
