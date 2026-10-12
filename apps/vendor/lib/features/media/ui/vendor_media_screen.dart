import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../onboarding/data/document_upload_client.dart';
import '../../orders/ui/order_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';

class MediaSave {
  const MediaSave(this.api, this.method, this.path, this.parameters, this.body);
  final VendorFeatureApi api;
  final String method, path;
  final Map<String, String> parameters;
  final JsonObject body;
}

Future<List<JsonObject>?> vendorMedia(
  BuildContext context, {
  required VendorRuntime runtime,
  required String targetType,
  required String targetRef,
  required List<JsonObject> entries,
  Future<MediaSave> Function(List<JsonObject>)? save,
}) => orderPage<List<JsonObject>>(
  context,
  VendorMediaScreen(
    runtime: runtime,
    targetType: targetType,
    targetRef: targetRef,
    entries: entries,
    save: save,
  ),
);

class VendorMediaScreen extends StatefulWidget {
  const VendorMediaScreen({
    super.key,
    required this.runtime,
    required this.targetType,
    required this.targetRef,
    required this.entries,
    this.save,
  });
  final VendorRuntime runtime;
  final String targetType, targetRef;
  final List<JsonObject> entries;
  final Future<MediaSave> Function(List<JsonObject>)? save;
  @override
  State<VendorMediaScreen> createState() => _VendorMediaScreenState();
}

class _VendorMediaScreenState extends State<VendorMediaScreen> {
  late final runtime = widget.runtime;
  late final backend = runtime.backend;
  late final session = backend.auth.session!;
  late final prefs = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final journalKey =
      'vendor.media.${sha256.convert(utf8.encode('${runtime.mode.name}|${backend.client.config.origin}|${session.vendorId}|${session.staffId}|${widget.targetType}|${widget.targetRef}'))}';
  late List<JsonObject> entries = widget.entries
      .map((e) => JsonObject.from(e))
      .toList();
  JsonObject pending = {};
  Uint8List? selected;
  String? mime, error;
  String phase = '';
  bool busy = false,
      loading = true,
      changed = false,
      ready = false,
      uploaded = false;
  bool get current =>
      identical(runtime.backend, backend) &&
      backend.auth.session?.staffId == session.staffId &&
      backend.auth.session?.vendorId == session.vendorId;
  bool get brand => widget.targetType == 'BRAND';
  bool get pastry => widget.targetType == 'PASTRY_REQUEST';
  Map<String, String> get target => {
    'target_type': widget.targetType,
    'target_ref': widget.targetRef,
  };
  @override
  void initState() {
    super.initState();
    restore();
  }

  Future<void> persistDraft() async {
    await prefs.write(
      "$journalKey.draft",
      jsonEncode({"initial": widget.entries, "entries": entries}),
    );
  }

  Future<void> persist() async {
    await prefs.write(journalKey, jsonEncode(pending));
  }

  Future<void> restore() async {
    try {
      final draft = await prefs.read('$journalKey.draft');
      if (draft != null && draft.isNotEmpty) {
        final value = jsonObject(jsonDecode(draft));
        if (jsonEncode(value['initial']) == jsonEncode(widget.entries)) {
          entries = (value['entries'] as List).map(jsonObject).toList();
          changed = true;
        }
      }
      final raw = await prefs.read(journalKey);
      if (raw != null && raw.isNotEmpty) pending = jsonObject(jsonDecode(raw));
      if (pending['media_ref'] != null) await check();
      if (pending['save'] != null) {
        entries = (pending['entries'] as List).map(jsonObject).toList();
        changed = true;
        phase = 'A previous save needs confirmation. Retry to confirm it.';
      }
    } catch (e) {
      error = message(e);
    }
    if (mounted) setState(() => loading = false);
  }

  String message(Object e) => e is ApiException
      ? e.message
      : e is VendorContractException
      ? e.message
      : e is FormatException
      ? e.message
      : 'Could not complete this action. Try again.';
  Future<void> check() async {
    final response = await backend.businessApi.request(
      'GET',
      '/api/v1/vendor/media/objects/{mediaRef}',
      pathParameters: {'mediaRef': pending['media_ref'].toString()},
      query: target,
    );
    if (!current) {
      throw const VendorContractException(
        'Your session changed. Reopen media.',
      );
    }
    final status = response.object;
    ready = status['usable'] == true;
    uploaded = status['upload_status'] == 'COMPLETED';
    mime = status['mime_type']?.toString();
    if (['UNSAFE', 'QUARANTINED'].contains(status['safety_status'])) {
      phase = 'This file was rejected by the safety check.';
    } else if (ready) {
      phase = 'Ready to add';
    } else if (status['safety_status'] == 'UNKNOWN') {
      phase =
          'File verification is unavailable. Check again later or replace this file.';
    } else if (uploaded) {
      phase = 'Checking this file. Refresh its status before adding it.';
    } else {
      phase = 'Upload is incomplete. Choose the same file to resume.';
    }
    if (ready && selected == null) {
      selected = await backend.client.mediaFile(
        pending['media_ref'].toString(),
        widget.targetType,
        widget.targetRef,
      );
    }
  }

  Future<void> refresh() async {
    if (busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await check();
    } catch (e) {
      error = message(e);
    }
    if (mounted) setState(() => busy = false);
  }

  Future<void> choose({bool example = false}) async {
    if (busy || !current || pending['save'] != null) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      Uint8List bytes;
      String name, type;
      if (example) {
        final path = brand
            ? 'assets/branding/karryam-logo.png'
            : 'assets/catalogue/pizza.jpg';
        bytes = (await rootBundle.load(path)).buffer.asUint8List();
        name = path.split('/').last;
        type = brand ? 'image/png' : 'image/jpeg';
      } else {
        final file = await FilePicker.platform.pickFiles(
          type: FileType.custom,
          allowedExtensions: ['jpg', 'jpeg', 'png', 'webp', if (pastry) 'pdf'],
          withData: true,
        );
        if (file == null) return;
        final picked = file.files.single;
        if (picked.bytes == null) {
          throw const FormatException('Could not read this file.');
        }
        bytes = picked.bytes!;
        name = picked.name;
        type = switch (picked.extension?.toLowerCase()) {
          'png' => 'image/png',
          'webp' => 'image/webp',
          'pdf' => 'application/pdf',
          'jpg' || 'jpeg' => 'image/jpeg',
          _ => '',
        };
      }
      if (!current) {
        throw const VendorContractException(
          'Your session changed. Reopen media.',
        );
      }
      if (bytes.isEmpty || bytes.length > 10 * 1024 * 1024 || type.isEmpty) {
        throw const FormatException(
          'Choose an image or supported document up to 10 MB.',
        );
      }
      final checksum = sha256.convert(bytes).toString();
      if (pending.isNotEmpty && pending['checksum'] != checksum) {
        throw const FormatException(
          'Resume with the original file, or discard this upload first.',
        );
      }
      selected = bytes;
      mime = type;
      if (pending.isEmpty) {
        pending = {
          'checksum': checksum,
          'key': VendorCommand.create().idempotencyKey,
          'request': {
            ...target,
            'file_name': name,
            'mime_type': type,
            'byte_size': bytes.length,
          },
        };
        await persist();
      }
      if (pending['media_ref'] != null) {
        await check();
        if (ready || uploaded) return;
      }
      phase = 'Preparing upload';
      if (mounted) setState(() {});
      final response = await backend.businessApi.request(
        'POST',
        '/api/v1/vendor/media/uploads',
        body: VendorPayload(jsonObject(pending['request'])),
        command: VendorCommand(idempotencyKey: pending['key'] as String),
      );
      if (!current) {
        throw const VendorContractException(
          'Your session changed. Reopen media.',
        );
      }
      final grant = response.object;
      pending['media_ref'] = grant['media_ref'];
      await persist();
      phase = 'Uploading file';
      if (mounted) setState(() {});
      await DocumentUploadClient().upload(
        grant,
        bytes,
        type,
        transport: runtime.mockUploadTransport,
      );
      await check();
    } catch (e) {
      error = message(e);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> discard() async {
    if (busy || pending['save'] != null) return;
    pending = {};
    await persist();
    if (mounted) {
      setState(() {
        selected = null;
        ready = false;
        uploaded = false;
        phase = '';
        error = null;
      });
    }
  }

  Future<void> add() async {
    if (!ready || busy || !current) return;
    if (!brand && entries.length >= (pastry ? 20 : 12)) {
      setState(
        () => error = pastry
            ? 'Attach at most 20 reference files.'
            : 'Attach at most 12 photos.',
      );
      return;
    }
    String kind = 'GALLERY';
    if (brand) {
      final chosen = await showModalBottomSheet<String>(
        context: context,
        showDragHandle: true,
        builder: (context) => SafeArea(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (final item in [
                ('LOGO', 'Brand logo', Icons.verified_outlined),
                ('COVER', 'Cover image', Icons.panorama_outlined),
                ('GALLERY', 'Gallery photo', Icons.photo_library_outlined),
              ])
                ListTile(
                  leading: Icon(item.$3, color: KarryamColors.teal),
                  title: Text(item.$2),
                  onTap: () => Navigator.pop(context, item.$1),
                ),
              const SizedBox(height: 16),
            ],
          ),
        ),
      );
      if (chosen == null || !mounted) return;
      kind = chosen;
    }
    final ref = pending['media_ref'].toString();
    setState(() {
      if (brand && kind != 'GALLERY') {
        entries.removeWhere((e) => e['kind'] == kind);
      }
      entries.removeWhere((e) => e['media_ref'] == ref);
      entries.add({
        'media_ref': ref,
        if (brand) 'kind': kind,
        if (widget.targetType == 'FOOD_ITEM') 'is_primary': entries.isEmpty,
      });
      changed = true;
      ready = false;
    });
    try {
      await persistDraft();
      pending = {};
      await persist();
    } catch (e) {
      if (mounted) {
        setState(
          () => error =
              'Could not save the draft. Keep this screen open until you save.',
        );
      }
      return;
    }
    uploaded = false;
    if (mounted) {
      setState(() {
        selected = null;
        phase = '';
      });
    }
  }

  Future<void> rememberEdit() async {
    try {
      await persistDraft();
    } catch (e) {
      if (mounted) {
        setState(
          () => error =
              'Could not save this draft. Keep this screen open until you save.',
        );
      }
    }
  }

  Future<void> save() async {
    if (busy || !current || !changed) return;
    if (widget.save == null) {
      await prefs.write('$journalKey.draft', '');
      if (mounted) Navigator.pop(context, entries);
      return;
    }
    setState(() {
      busy = true;
      error = null;
      phase = 'Saving media';
    });
    try {
      MediaSave request;
      if (pending['save'] == null) {
        request = await widget.save!(entries);
        pending = {
          'entries': entries,
          'save': {
            'method': request.method,
            'path': request.path,
            'parameters': request.parameters,
            'body': request.body,
            'key': VendorCommand.create().idempotencyKey,
          },
        };
        await persist();
      } else {
        final s = jsonObject(pending['save']);
        final expectedPath = switch (widget.targetType) {
          'FOOD_ITEM' => '/api/v1/vendor/food/items/{itemId}/media',
          'SHOP_PRODUCT' => '/api/v1/vendor/shop/products/{productId}',
          _ => '/api/v1/vendor/businesses/{businessId}/brands/{brandId}',
        };
        final idKey = switch (widget.targetType) {
          'FOOD_ITEM' => 'itemId',
          'SHOP_PRODUCT' => 'productId',
          _ => 'brandId',
        };
        if (s['path'] != expectedPath ||
            (s['parameters'] as Map)[idKey] != widget.targetRef) {
          throw const VendorContractException(
            'Saved media action does not match this record.',
          );
        }
        final api = switch (widget.targetType) {
          'FOOD_ITEM' => backend.foodCatalogueApi,
          'SHOP_PRODUCT' => backend.shopCatalogueApi,
          _ => backend.businessApi,
        };
        request = MediaSave(
          api,
          s['method'] as String,
          s['path'] as String,
          Map<String, String>.from(s['parameters'] as Map),
          jsonObject(s['body']),
        );
      }
      if (!current) {
        throw const VendorContractException(
          'Your session changed. Reopen media.',
        );
      }
      await request.api.request(
        request.method,
        request.path,
        pathParameters: request.parameters,
        body: VendorPayload(request.body),
        command: VendorCommand(
          idempotencyKey: jsonObject(pending['save'])['key'] as String,
        ),
      );
      await prefs.write('$journalKey.draft', '');
      pending = {};
      await persist();
      if (!mounted || !current) return;
      setState(() {
        busy = false;
        phase = '';
      });
      await showOrderFeedback(
        context,
        title: 'Media saved',
        message: brand
            ? 'Your brand images are updated. Review any updated verification requirements in Business setup.'
            : 'Your product photos are updated.',
        success: true,
      );
      if (mounted) Navigator.pop(context, entries);
    } catch (e) {
      error = message(e);
      if (e is ApiException &&
          (e.statusCode ?? 0) >= 400 &&
          (e.statusCode ?? 0) < 500 &&
          ![408, 429].contains(e.statusCode)) {
        pending = {};
        await persist();
      }
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !busy,
    child: Scaffold(
      appBar: AppBar(
        title: Text(
          brand
              ? 'Brand images'
              : pastry
              ? 'Reference files'
              : 'Product photos',
        ),
      ),
      body: loading
          ? const Padding(
              padding: EdgeInsets.all(24),
              child: PremiumSkeleton(rows: 3),
            )
          : ListView(
              padding: const EdgeInsets.fromLTRB(24, 12, 24, 32),
              children: [
                Text(
                  brand
                      ? 'Make your brand recognisable'
                      : pastry
                      ? 'Bring the design to life'
                      : 'Let your photos do the talking',
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                    fontWeight: FontWeight.w700,
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  pastry
                      ? 'Images or PDF references. Up to 10 MB each.'
                      : 'JPG, PNG or WebP. Up to 10 MB each.',
                  style: Theme.of(context).textTheme.bodyMedium,
                ),
                const SizedBox(height: 24),
                if (entries.isNotEmpty) ...[
                  LayoutBuilder(
                    builder: (context, constraints) => Wrap(
                      spacing: 12,
                      runSpacing: 12,
                      children: [
                        for (final e in entries)
                          SizedBox(
                            width: (constraints.maxWidth - 12) / 2,
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                AspectRatio(
                                  aspectRatio: 1,
                                  child: ClipRRect(
                                    borderRadius: BorderRadius.circular(20),
                                    child: MediaPreview(
                                      runtime: runtime,
                                      targetType: widget.targetType,
                                      targetRef: widget.targetRef,
                                      ref: e['media_ref'].toString(),
                                    ),
                                  ),
                                ),
                                Row(
                                  children: [
                                    Expanded(
                                      child: Text(
                                        brand
                                            ? '${e['kind']}'
                                            : e['is_primary'] == true
                                            ? 'Primary photo'
                                            : pastry
                                            ? 'Reference file'
                                            : 'Photo',
                                        style: const TextStyle(
                                          fontWeight: FontWeight.w600,
                                        ),
                                      ),
                                    ),
                                    IconButton(
                                      tooltip: 'Remove from this record',
                                      onPressed: busy || pending['save'] != null
                                          ? null
                                          : () => setState(() {
                                              entries.remove(e);
                                              if (widget.targetType ==
                                                      'FOOD_ITEM' &&
                                                  entries.isNotEmpty &&
                                                  !entries.any(
                                                    (v) =>
                                                        v['is_primary'] == true,
                                                  )) {
                                                entries.first['is_primary'] =
                                                    true;
                                              }
                                              changed = true;
                                              rememberEdit();
                                            }),
                                      icon: Icon(
                                        Icons.close_rounded,
                                        color: Theme.of(
                                          context,
                                        ).colorScheme.error,
                                      ),
                                    ),
                                  ],
                                ),
                                if (widget.targetType == 'FOOD_ITEM' &&
                                    e['is_primary'] != true)
                                  TextButton(
                                    onPressed: busy || pending['save'] != null
                                        ? null
                                        : () => setState(() {
                                            for (final row in entries) {
                                              row['is_primary'] = identical(
                                                row,
                                                e,
                                              );
                                            }
                                            changed = true;
                                            rememberEdit();
                                          }),
                                    child: const Text('Make primary'),
                                  ),
                              ],
                            ),
                          ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 20),
                ],
                if (selected != null) ...[
                  ClipRRect(
                    borderRadius: BorderRadius.circular(24),
                    child: AspectRatio(
                      aspectRatio: 1.3,
                      child: mime == 'application/pdf'
                          ? const _MediaPlaceholder(pdf: true)
                          : Image.memory(
                              selected!,
                              fit: BoxFit.contain,
                              errorBuilder: (_, _, _) =>
                                  const _MediaPlaceholder(),
                            ),
                    ),
                  ),
                  const SizedBox(height: 12),
                ],
                AnimatedSize(
                  duration: const Duration(milliseconds: 240),
                  child: Column(
                    children: [
                      if (phase.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 12),
                          child: Row(
                            children: [
                              if (busy)
                                const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                ),
                              if (busy) const SizedBox(width: 12),
                              Expanded(child: Text(phase)),
                            ],
                          ),
                        ),
                      if (error != null)
                        Container(
                          width: double.infinity,
                          padding: const EdgeInsets.all(16),
                          margin: const EdgeInsets.only(bottom: 16),
                          decoration: BoxDecoration(
                            color: Theme.of(context).colorScheme.errorContainer,
                            borderRadius: BorderRadius.circular(16),
                          ),
                          child: Text(
                            error!,
                            style: TextStyle(
                              color: Theme.of(
                                context,
                              ).colorScheme.onErrorContainer,
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
                if (ready)
                  FilledButton.icon(
                    onPressed: busy ? null : add,
                    icon: const Icon(Icons.add_photo_alternate_outlined),
                    label: const Text('Add this file'),
                  ),
                if (!ready && !uploaded && pending['save'] == null)
                  OutlinedButton.icon(
                    onPressed: busy ? null : () => choose(),
                    icon: const Icon(Icons.add_photo_alternate_outlined),
                    label: Text(
                      pending.isEmpty ? 'Choose file' : 'Resume upload',
                    ),
                  ),
                if (runtime.mode == VendorDataMode.mock &&
                    !ready &&
                    !uploaded &&
                    pending['save'] == null)
                  TextButton.icon(
                    onPressed: busy ? null : () => choose(example: true),
                    icon: const Icon(Icons.auto_awesome_outlined),
                    label: const Text('Use sample image'),
                  ),
                if (pending['media_ref'] != null && !ready)
                  TextButton.icon(
                    onPressed: busy ? null : refresh,
                    icon: const Icon(Icons.refresh_rounded),
                    label: const Text('Check file status'),
                  ),
                if (pending.isNotEmpty && pending['save'] == null)
                  TextButton(
                    onPressed: busy ? null : discard,
                    child: const Text('Discard upload'),
                  ),
                const SizedBox(height: 24),
                FilledButton(
                  onPressed: busy || !changed || pending['media_ref'] != null
                      ? null
                      : save,
                  child: Text(
                    pending['save'] != null
                        ? 'Retry save'
                        : pastry
                        ? 'Use references'
                        : 'Save media',
                  ),
                ),
              ],
            ),
    ),
  );
}

class MediaPreview extends StatefulWidget {
  const MediaPreview({
    super.key,
    required this.runtime,
    required this.targetType,
    required this.targetRef,
    required this.ref,
  });
  final VendorRuntime runtime;
  final String targetType, targetRef, ref;
  @override
  State<MediaPreview> createState() => _MediaPreviewState();
}

class _MediaPreviewState extends State<MediaPreview> {
  late Future<Uint8List> file = load();
  Future<Uint8List> load() => widget.runtime.backend.client.mediaFile(
    widget.ref,
    widget.targetType,
    widget.targetRef,
  );
  @override
  void didUpdateWidget(MediaPreview old) {
    super.didUpdateWidget(old);
    if (old.ref != widget.ref || old.targetRef != widget.targetRef) {
      file = load();
    }
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<Uint8List>(
    future: file,
    builder: (context, snapshot) {
      if (snapshot.connectionState != ConnectionState.done) {
        return const PremiumSkeleton(rows: 1);
      }
      if (snapshot.hasError) {
        return InkWell(
          onTap: () => setState(() => file = load()),
          child: const _MediaPlaceholder(retry: true),
        );
      }
      final bytes = snapshot.data!;
      final pdf =
          bytes.length >= 4 &&
          utf8.decode(bytes.take(4).toList(), allowMalformed: true) == '%PDF';
      return InkWell(
        onTap: () =>
            orderPage<void>(context, _MediaViewer(bytes: bytes, pdf: pdf)),
        child: pdf
            ? const _MediaPlaceholder(pdf: true)
            : Image.memory(
                bytes,
                fit: BoxFit.cover,
                errorBuilder: (_, _, _) => const _MediaPlaceholder(),
              ),
      );
    },
  );
}

class _MediaPlaceholder extends StatelessWidget {
  const _MediaPlaceholder({this.pdf = false, this.retry = false});
  final bool pdf, retry;
  @override
  Widget build(BuildContext context) => ColoredBox(
    color: Theme.of(context).colorScheme.surfaceContainerHighest,
    child: Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            pdf ? Icons.picture_as_pdf_outlined : Icons.image_outlined,
            size: 36,
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
          const SizedBox(height: 8),
          Text(
            pdf
                ? 'PDF reference'
                : retry
                ? 'Tap to retry preview'
                : 'Preview unavailable',
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    ),
  );
}

class _MediaViewer extends StatefulWidget {
  const _MediaViewer({required this.bytes, required this.pdf});
  final Uint8List bytes;
  final bool pdf;
  @override
  State<_MediaViewer> createState() => _MediaViewerState();
}

class _MediaViewerState extends State<_MediaViewer> {
  bool saving = false;
  Future<void> download() async {
    setState(() => saving = true);
    try {
      final path = await FilePicker.platform.saveFile(
        dialogTitle: 'Save reference',
        fileName: 'karryam-reference.pdf',
        bytes: widget.bytes,
        type: FileType.custom,
        allowedExtensions: ['pdf'],
      );
      if (path != null && mounted) {
        await showOrderFeedback(
          context,
          title: 'File saved',
          message: 'Your reference file is ready.',
          success: true,
        );
      }
    } catch (e) {
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Could not save file',
          message: 'Try saving the reference again.',
        );
      }
    } finally {
      if (mounted) setState(() => saving = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(widget.pdf ? 'PDF reference' : 'Photo')),
    body: widget.pdf
        ? Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                const SizedBox(
                  height: 160,
                  child: _MediaPlaceholder(pdf: true),
                ),
                const SizedBox(height: 24),
                FilledButton.icon(
                  onPressed: saving ? null : download,
                  icon: saving
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.download_rounded),
                  label: const Text('Save PDF'),
                ),
              ],
            ),
          )
        : Center(
            child: InteractiveViewer(
              minScale: .5,
              maxScale: 5,
              child: Image.memory(widget.bytes, fit: BoxFit.contain),
            ),
          ),
  );
}
