import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/provider_operations_api.dart';
import 'mission_photo_field.dart';
import 'parcel_scanner_screen.dart';

class HubCustodyScreen extends StatefulWidget {
  const HubCustodyScreen({super.key, required this.api, required this.tripId});
  final ProviderOperationsApi api;
  final String tripId;
  @override
  State<HubCustodyScreen> createState() => _HubCustodyScreenState();
}

class _HubCustodyScreenState extends State<HubCustodyScreen> {
  Map<String, dynamic>? _hub;
  String? _error, _phase;
  bool _loading = true;
  bool _refreshing = false;
  String? _savedStage;
  final _scans = <String>{};
  final _photo = MissionPhotoDraft();
  String get _path =>
      '/api/v1/provider/trips/${Uri.encodeComponent(widget.tripId)}/hub-hold';
  List<String> values(dynamic v) =>
      (v as List? ?? []).map((v) => v.toString()).toList();
  String get _stage => _hub?['allowed_stage']?.toString() ?? '';
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    if (_refreshing) return;
    setState(() => _refreshing = true);
    try {
      final data = await widget.api.read(_path);
      if (!mounted) return;
      setState(() {
        if (_hub?['manifest_ref'] != data['manifest_ref'] ||
            _hub?['allowed_stage'] != data['allowed_stage']) {
          _scans.clear();
          _photo.clear();
        }
        _hub = data;
      });
    } catch (error) {
      if (mounted)
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'Hub details could not be loaded. Try again.',
        );
    } finally {
      if (mounted)
        setState(() {
          _loading = false;
          _refreshing = false;
        });
    }
  }

  Future<void> _scan() async {
    final code = await Navigator.of(context).push<String>(
      MaterialPageRoute(builder: (_) => const ParcelScannerScreen()),
    );
    if (code == null || !mounted) return;
    setState(() {
      if (!values(_hub?['parcel_codes']).contains(code.trim())) {
        _error = 'This parcel is not in the active hub manifest.';
      } else {
        _scans.add(code.trim());
        _error = null;
      }
    });
  }

  Future<void> _submit() async {
    final expected = values(_hub?['parcel_codes']);
    if (_phase != null ||
        _stage.isEmpty ||
        _photo.path == null ||
        _scans.length != expected.length ||
        !expected.every(_scans.contains))
      return;
    final stage = _stage;
    setState(() {
      _phase = 'Uploading hub photo';
      _error = null;
    });
    try {
      final evidence = await _photo.attach(
        widget.api,
        '$_path/evidence-content',
        values(_hub!['parcel_refs']),
        {},
        fields: {'manifest_ref': _hub!['manifest_ref'] as String},
      );
      if (!mounted) return;
      setState(() => _phase = 'Recording checkpoint');
      await widget.api.command('$_path/checkpoints', {
        'stage': stage,
        'parcel_codes': _scans.toList()..sort(),
        'media_ref': (evidence['media_refs'] as List).single,
      });
      if (mounted) setState(() => _savedStage = stage);
    } catch (error) {
      if (mounted) {
        setState(
          () => _error = error is ApiException
              ? error.message
              : 'The checkpoint could not be confirmed. Refresh the hub status before retrying.',
        );
        await _load();
      }
    } finally {
      if (mounted) setState(() => _phase = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final expected = values(_hub?['parcel_codes']);
    final ready =
        _stage.isNotEmpty &&
        expected.isNotEmpty &&
        expected.every(_scans.contains) &&
        _photo.path != null &&
        !_photo.picking;
    return PopScope(
      canPop: _phase == null,
      child: Scaffold(
        appBar: AppBar(
          title: const Text('Hub custody'),
          actions: [
            IconButton(
              tooltip: 'Refresh hub status',
              onPressed: _phase == null && !_refreshing
                  ? () async {
                      setState(() => _error = null);
                      await _load();
                    }
                  : null,
              icon: const Icon(Icons.refresh_rounded),
            ),
          ],
        ),
        body: _loading
            ? const ContentSkeleton()
            : _savedStage != null
            ? ConfirmedResult(
                title: _savedStage == 'ARRIVAL'
                    ? 'Hub arrival recorded'
                    : 'Hub release recorded',
                message: _savedStage == 'ARRIVAL'
                    ? 'The parcels are recorded as held at the approved hub.'
                    : 'The parcels are recorded as released from the hub.',
                onDone: () => Navigator.pop(context, true),
              )
            : ListView(
                padding: const EdgeInsets.all(20),
                children: [
                  if (_error != null) ...[
                    ActionNotice(
                      message: _error!,
                      isError: true,
                      onRetry: _phase == null ? _load : null,
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (_hub != null) ...[
                    Icon(
                      Icons.warehouse_outlined,
                      size: 48,
                      color: c.brandTeal600,
                    ),
                    const SizedBox(height: 16),
                    Text(
                      _stage == 'ARRIVAL'
                          ? 'Arrive at the approved hub'
                          : _stage == 'RELEASE'
                          ? 'Collect from the hub'
                          : 'Hub custody status',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 12),
                    Card(
                      child: ListTile(
                        title: const Text('Approved hub'),
                        subtitle: SelectableText(
                          _hub!['hub_ref']?.toString() ?? '',
                        ),
                        trailing: Text(switch (_hub!['state']) {
                          'HELD' => 'Held',
                          'RELEASED' => 'Released',
                          'PICKUP_PENDING' => 'Pickup pending',
                          'APPROVED' => 'Approved',
                          'EN_ROUTE' => 'On the way',
                          'CUSTODY_PENDING' => 'Check-in pending',
                          'CANCELLED' => 'Cancelled',
                          _ => 'Status unavailable',
                        }),
                      ),
                    ),
                    const SizedBox(height: 20),
                    if (_stage.isEmpty)
                      const ActionNotice(
                        message:
                            'There is no hub checkpoint to record at this stage.',
                      )
                    else ...[
                      Text(
                        '${_scans.length} of ${expected.length} parcels scanned',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 12),
                      for (final entry in expected.asMap().entries)
                        Card(
                          child: ListTile(
                            leading: Icon(
                              _scans.contains(entry.value)
                                  ? Icons.task_alt_rounded
                                  : Icons.qr_code_2_rounded,
                              color: _scans.contains(entry.value)
                                  ? c.success
                                  : c.textMuted,
                            ),
                            title: Text('Parcel ${entry.key + 1}'),
                            subtitle: Text(
                              _scans.contains(entry.value)
                                  ? 'Scanned'
                                  : 'Scan required',
                            ),
                          ),
                        ),
                      OutlinedButton.icon(
                        onPressed: _phase == null && !_refreshing
                            ? _scan
                            : null,
                        icon: const Icon(Icons.qr_code_scanner_rounded),
                        label: const Text('Scan parcel'),
                      ),
                      const SizedBox(height: 20),
                      MissionPhotoField(
                        draft: _photo,
                        required: true,
                        enabled: _phase == null && !_refreshing,
                        onChanged: () => setState(() {}),
                      ),
                      const SizedBox(height: 24),
                      FilledButton.icon(
                        onPressed: ready && _phase == null && !_refreshing
                            ? _submit
                            : null,
                        icon: _phase == null
                            ? const Icon(Icons.check_rounded)
                            : const SizedBox(
                                width: 18,
                                height: 18,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                ),
                              ),
                        label: Text(
                          _phase ??
                              (_stage == 'ARRIVAL'
                                  ? 'Record hub arrival'
                                  : 'Record hub release'),
                        ),
                      ),
                    ],
                  ],
                ],
              ),
      ),
    );
  }
}
