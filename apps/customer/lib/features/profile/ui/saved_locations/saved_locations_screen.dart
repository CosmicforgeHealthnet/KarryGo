import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../../auth/data/customer_auth_api.dart';
import '../../../auth/models/customer_auth_models.dart';
import '../../../hauling/data/places_api.dart';
import 'saved_location_form_screen.dart';
import 'saved_locations_header.dart';

/// Figma 1777: list of saved locations with a "+ Add New" pill and
/// tappable cards. Tap a card to edit; long-press to delete.
class SavedLocationsScreen extends StatefulWidget {
  const SavedLocationsScreen({
    super.key,
    required this.session,
    required this.api,
    required this.placesApi,
  });

  final CustomerSession session;
  final CustomerAuthApi api;
  final PlacesApi placesApi;

  @override
  State<SavedLocationsScreen> createState() => _SavedLocationsScreenState();
}

class _SavedLocationsScreenState extends State<SavedLocationsScreen> {
  List<SavedLocation> _locations = [];
  bool _loading = true;
  ApiException? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final locations = await widget.api.getSavedLocations(
        accessToken: widget.session.accessToken,
      );
      if (!mounted) return;
      setState(() {
        _locations = locations;
        _loading = false;
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _openForm({SavedLocation? existing}) async {
    final result = await Navigator.of(context).push<Object>(
      MaterialPageRoute(
        builder: (_) => SavedLocationFormScreen(
          session: widget.session,
          api: widget.api,
          placesApi: widget.placesApi,
          existing: existing,
        ),
      ),
    );
    // Saved (pops a SavedLocation) or deleted (pops true) — either way reload.
    if (result != null) _load();
  }

  Future<void> _confirmDelete(SavedLocation location) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete Location'),
        content: Text('Remove "${location.title}" from your saved locations?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child:
                const Text('Delete', style: TextStyle(color: Color(0xFFE53935))),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;

    try {
      await widget.api.deleteSavedLocation(
        accessToken: widget.session.accessToken,
        id: location.id,
      );
      if (!mounted) return;
      setState(() => _locations.removeWhere((l) => l.id == location.id));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(e.message)));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SavedLocationsHeader(),
            const SizedBox(height: 16),
            Padding(
              padding: const EdgeInsets.only(right: 20),
              child: Align(
                alignment: Alignment.centerRight,
                child: _AddNewButton(onTap: () => _openForm()),
              ),
            ),
            const SizedBox(height: 14),
            Expanded(child: _buildBody()),
          ],
        ),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const Center(
        child: CircularProgressIndicator(color: CustomerFigmaColors.primary),
      );
    }
    final error = _error;
    if (error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline_rounded,
                  color: Color(0xFFE53935), size: 40),
              const SizedBox(height: 16),
              Text(
                error.message,
                textAlign: TextAlign.center,
                style: const TextStyle(
                    color: CustomerFigmaColors.text, fontSize: 14),
              ),
              const SizedBox(height: 20),
              FigmaPrimaryButton(label: 'Try again', onPressed: _load),
            ],
          ),
        ),
      );
    }
    if (_locations.isEmpty) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.map_outlined, color: CustomerFigmaColors.muted, size: 40),
              SizedBox(height: 12),
              Text(
                'No saved locations yet.\nAdd one to book trips faster.',
                textAlign: TextAlign.center,
                style:
                    TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
              ),
            ],
          ),
        ),
      );
    }
    return RefreshIndicator(
      color: CustomerFigmaColors.primary,
      onRefresh: _load,
      child: ListView.separated(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(16, 2, 16, 24),
        itemCount: _locations.length,
        separatorBuilder: (_, _) => const SizedBox(height: 13),
        itemBuilder: (context, i) {
          final location = _locations[i];
          return _SavedLocationCard(
            location: location,
            onTap: () => _openForm(existing: location),
            onLongPress: () => _confirmDelete(location),
          );
        },
      ),
    );
  }
}

// ─── "+ Add New" pill ─────────────────────────────────────────────────────────

class _AddNewButton extends StatelessWidget {
  const _AddNewButton({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: CustomerFigmaColors.primary,
      borderRadius: BorderRadius.circular(999),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(999),
        child: const Padding(
          padding: EdgeInsets.symmetric(horizontal: 16, vertical: 9),
          child: Text(
            '+ Add New',
            style: TextStyle(
              color: Colors.white,
              fontSize: 13,
              fontWeight: FontWeight.w700,
            ),
          ),
        ),
      ),
    );
  }
}

// ─── Saved location card ──────────────────────────────────────────────────────

class _SavedLocationCard extends StatelessWidget {
  const _SavedLocationCard({
    required this.location,
    required this.onTap,
    required this.onLongPress,
  });

  final SavedLocation location;
  final VoidCallback onTap;
  final VoidCallback onLongPress;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.white,
      borderRadius: BorderRadius.circular(12),
      elevation: 2,
      shadowColor: const Color(0x24000000),
      child: InkWell(
        onTap: onTap,
        onLongPress: onLongPress,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      location.title,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 4),
                    Text(
                      location.address,
                      style: const TextStyle(
                        color: CustomerFigmaColors.muted,
                        fontSize: 13,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              const Icon(Icons.chevron_right_rounded,
                  color: CustomerFigmaColors.muted, size: 22),
            ],
          ),
        ),
      ),
    );
  }
}
