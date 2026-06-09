// lib/shared/widgets/document_upload_box.dart
//
// Reusable document upload widget.
// Supports: Take Photo (camera), Choose Image (gallery), Choose PDF.
// Allowed types for backend: jpg, jpeg, png, pdf.
//
// TODO – backend upload mapping:
//   Government ID  : POST /api/v1/provider/verification/identity
//                    fields: govt_id_type, govt_id_number, govt_id_file, profile_photo
//   Driver Licence : POST /api/v1/provider/verification/licence
//                    fields: licence_number, expiry_year, expiry_month, licence_file
//   Vehicle Docs   : POST /api/v1/provider/vehicle/:id/documents
//                    fields: document_type, expiry_date (optional), document_file

import 'dart:io';
import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';
import 'package:file_picker/file_picker.dart';
import 'package:path/path.dart' as p;

/// Callback type: (filePath, fileName)
typedef OnFileSelected = void Function(String filePath, String fileName);

// ── Allowed extensions ────────────────────────────────────────────────────────

const _allowedImageExts = {'jpg', 'jpeg', 'png'};
const _allowedExts = {'jpg', 'jpeg', 'png', 'pdf'};

// ── DocumentUploadBox ─────────────────────────────────────────────────────────

class DocumentUploadBox extends StatefulWidget {
  const DocumentUploadBox({
    super.key,
    required this.onFileSelected,
    this.filePath,
    this.fileName,
    this.onClear,
    this.hint = 'Supported file type: JPG, PNG, PDF\nMaximum File Size: 500 MB',
  });

  /// Called when the user selects a file.
  final OnFileSelected onFileSelected;

  /// Optional: currently selected file path (image or pdf).
  final String? filePath;

  /// Optional: display name for the currently selected file.
  final String? fileName;

  /// Optional: called when user clears selection.
  final VoidCallback? onClear;

  /// Hint text displayed below the upload box.
  final String hint;

  @override
  State<DocumentUploadBox> createState() => _DocumentUploadBoxState();
}

class _DocumentUploadBoxState extends State<DocumentUploadBox> {
  final _picker = ImagePicker();

  bool get _isImage =>
      widget.fileName != null &&
      _allowedImageExts.contains(
          widget.fileName!.split('.').last.toLowerCase());

  // ── picker methods ──────────────────────────────────────────────────────────

  Future<void> _pickCamera() async {
    try {
      final XFile? file = await _picker.pickImage(
        source: ImageSource.camera,
        imageQuality: 85,
        maxWidth: 1920,
        maxHeight: 1920,
      );
      if (!mounted) return;
      if (file == null) return; // user cancelled
      final ext = p.extension(file.path).replaceFirst('.', '').toLowerCase();
      if (!_allowedImageExts.contains(ext)) {
        _showError('Unsupported file type: .$ext');
        return;
      }
      widget.onFileSelected(file.path, p.basename(file.path));
    } catch (e) {
      if (!mounted) return;
      _showError('Camera unavailable. Please grant camera permission in Settings.');
    }
  }

  Future<void> _pickGallery() async {
    try {
      final XFile? file = await _picker.pickImage(
        source: ImageSource.gallery,
        imageQuality: 85,
        maxWidth: 1920,
        maxHeight: 1920,
      );
      if (!mounted) return;
      if (file == null) return; // user cancelled
      final ext = p.extension(file.path).replaceFirst('.', '').toLowerCase();
      if (!_allowedImageExts.contains(ext)) {
        _showError('Unsupported file type: .$ext');
        return;
      }
      widget.onFileSelected(file.path, p.basename(file.path));
    } catch (e) {
      if (!mounted) return;
      _showError('Gallery unavailable. Please grant photo permission in Settings.');
    }
  }

  Future<void> _pickPdf() async {
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['pdf'],
        allowMultiple: false,
      );
      if (!mounted) return;
      if (result == null || result.files.isEmpty) return; // user cancelled
      final pf = result.files.first;
      final path = pf.path;
      if (path == null) {
        _showError('Could not access the selected PDF file.');
        return;
      }
      final ext = (pf.extension ?? '').toLowerCase();
      if (!_allowedExts.contains(ext)) {
        _showError('Unsupported file type: .$ext');
        return;
      }
      widget.onFileSelected(path, pf.name);
    } catch (e) {
      if (!mounted) return;
      _showError('Could not open file picker. Please try again.');
    }
  }

  // ── bottom sheet ────────────────────────────────────────────────────────────

  void _showPickerSheet() {
    showModalBottomSheet<void>(
      context: context,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 36,
                height: 4,
                decoration: BoxDecoration(
                  color: const Color(0xFFDDDDDD),
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
              const SizedBox(height: 20),
              const Text(
                'Choose an option',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              const SizedBox(height: 20),
              _SheetOption(
                icon: Icons.camera_alt_outlined,
                label: 'Take Photo',
                onTap: () {
                  Navigator.pop(ctx);
                  _pickCamera();
                },
              ),
              const Divider(height: 1),
              _SheetOption(
                icon: Icons.photo_library_outlined,
                label: 'Choose Image from Gallery',
                onTap: () {
                  Navigator.pop(ctx);
                  _pickGallery();
                },
              ),
              const Divider(height: 1),
              _SheetOption(
                icon: Icons.picture_as_pdf_outlined,
                label: 'Choose PDF Document',
                onTap: () {
                  Navigator.pop(ctx);
                  _pickPdf();
                },
              ),
              const SizedBox(height: 8),
            ],
          ),
        ),
      ),
    );
  }

  // ── error helper ────────────────────────────────────────────────────────────

  void _showError(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: const Color(0xFFE53935),
      ),
    );
  }

  // ── build ───────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _buildBox(),
        const SizedBox(height: 8),
        Text(
          widget.hint,
          style: const TextStyle(
            fontSize: 11,
            color: Color(0xFFAAAAAA),
            height: 1.6,
          ),
        ),
      ],
    );
  }

  Widget _buildBox() {
    final hasFile = widget.filePath != null && widget.fileName != null;

    // ── Selected state ──────────────────────────────────────────────────────
    if (hasFile) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: const Color(0xFFE0E0E0)),
        ),
        child: Row(
          children: [
            // Preview: image thumbnail or PDF icon
            if (_isImage)
              ClipRRect(
                borderRadius: BorderRadius.circular(6),
                child: Image.file(
                  File(widget.filePath!),
                  width: 40,
                  height: 40,
                  fit: BoxFit.cover,
                ),
              )
            else
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: const Color(0xFFFFEBEE),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: const Icon(
                  Icons.picture_as_pdf_outlined,
                  size: 22,
                  color: Color(0xFFE53935),
                ),
              ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    widget.fileName!,
                    style: const TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                      color: Color(0xFF1A1A1A),
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                  const SizedBox(height: 2),
                  GestureDetector(
                    onTap: _showPickerSheet,
                    child: const Text(
                      'Tap to change',
                      style: TextStyle(
                        fontSize: 11,
                        color: Color(0xFF4CAF50),
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            GestureDetector(
              onTap: widget.onClear,
              child: const Icon(
                Icons.cancel_outlined,
                size: 22,
                color: Color(0xFF888888),
              ),
            ),
          ],
        ),
      );
    }

    // ── Empty state — dashed border prompt ──────────────────────────────────
    return GestureDetector(
      onTap: _showPickerSheet,
      child: CustomPaint(
        painter: _DashedBorderPainter(
          color: const Color(0xFFCCCCCC),
          borderRadius: 10,
          dashWidth: 6,
          dashSpace: 4,
        ),
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(vertical: 24),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(10),
          ),
          child: const Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.upload_file_rounded,
                size: 30,
                color: Color(0xFF4CAF50),
              ),
              SizedBox(height: 8),
              Text(
                'Upload Document',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              SizedBox(height: 4),
              Text(
                'Tap to take photo, choose image or PDF',
                style: TextStyle(
                  fontSize: 11,
                  color: Color(0xFF888888),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ── Sheet option row ──────────────────────────────────────────────────────────

class _SheetOption extends StatelessWidget {
  const _SheetOption({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 14),
        child: Row(
          children: [
            Icon(icon, size: 22, color: const Color(0xFF4CAF50)),
            const SizedBox(width: 16),
            Text(
              label,
              style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: Color(0xFF1A1A1A),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Dashed border painter ─────────────────────────────────────────────────────

class _DashedBorderPainter extends CustomPainter {
  const _DashedBorderPainter({
    required this.color,
    required this.borderRadius,
    required this.dashWidth,
    required this.dashSpace,
  });

  final Color color;
  final double borderRadius;
  final double dashWidth;
  final double dashSpace;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final path = Path()
      ..addRRect(RRect.fromRectAndRadius(
        Rect.fromLTWH(0, 0, size.width, size.height),
        Radius.circular(borderRadius),
      ));

    final dashPath = Path();
    for (final metric in path.computeMetrics()) {
      double distance = 0;
      while (distance < metric.length) {
        dashPath.addPath(
          metric.extractPath(distance, distance + dashWidth),
          Offset.zero,
        );
        distance += dashWidth + dashSpace;
      }
    }
    canvas.drawPath(dashPath, paint);
  }

  @override
  bool shouldRepaint(_DashedBorderPainter old) =>
      old.color != color ||
      old.dashWidth != dashWidth ||
      old.dashSpace != dashSpace;
}
