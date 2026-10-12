import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'dart:io';

Future<bool> saveExportFile(Uint8List bytes, String name) async {
  final path = await FilePicker.platform.saveFile(
    dialogTitle: 'Save export',
    fileName: name,
    bytes: bytes,
    type: FileType.custom,
    allowedExtensions: [name.endsWith('.csv') ? 'csv' : 'jsonl'],
  );
  if (path == null) return false;
  if (Platform.isWindows || Platform.isLinux || Platform.isMacOS) {
    await File(path).writeAsBytes(bytes, flush: true);
  }
  return true;
}
