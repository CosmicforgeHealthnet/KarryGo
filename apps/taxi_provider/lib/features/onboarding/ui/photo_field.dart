import 'package:flutter/material.dart';
import 'package:image_picker/image_picker.dart';

import '../../../core/theme/karryam_theme.dart';

/// A tile that takes or chooses one photo. Shows a tick once chosen (or when
/// a photo is already on file).
class PhotoField extends StatelessWidget {
  const PhotoField({
    super.key,
    required this.label,
    required this.file,
    required this.onPicked,
    this.alreadyOnFile = false,
  });

  final String label;
  final XFile? file;
  final ValueChanged<XFile> onPicked;
  final bool alreadyOnFile;

  Future<void> _pick(BuildContext context) async {
    final source = await showModalBottomSheet<ImageSource>(
      context: context,
      builder: (c) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.photo_camera_outlined),
              title: const Text('Take a photo'),
              onTap: () => Navigator.pop(c, ImageSource.camera),
            ),
            ListTile(
              leading: const Icon(Icons.photo_library_outlined),
              title: const Text('Choose from gallery'),
              onTap: () => Navigator.pop(c, ImageSource.gallery),
            ),
          ],
        ),
      ),
    );
    if (source == null) return;
    // Scaled down so a document photo stays well under the 10 MB limit.
    final picked = await ImagePicker().pickImage(
      source: source,
      maxWidth: 1800,
      imageQuality: 80,
    );
    if (picked != null) onPicked(picked);
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    final done = file != null || alreadyOnFile;
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Material(
        color: colors.card,
        borderRadius: BorderRadius.circular(12),
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: () => _pick(context),
          child: Container(
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: done ? colors.primary : colors.border),
            ),
            child: Row(
              children: [
                Icon(
                  done
                      ? Icons.check_circle_rounded
                      : Icons.add_a_photo_outlined,
                  color: done ? colors.primary : colors.textMuted,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        label,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                      Text(
                        file != null
                            ? 'Photo ready to upload'
                            : alreadyOnFile
                            ? 'On file. Tap to replace'
                            : 'Tap to add a photo',
                        style: TextStyle(color: colors.textMuted, fontSize: 12),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
