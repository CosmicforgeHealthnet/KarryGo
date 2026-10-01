import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../data/trip_model.dart';

class ProofOfDeliveryScreen extends StatefulWidget {
  const ProofOfDeliveryScreen({super.key, required this.trip});
  final TripModel trip;

  @override
  State<ProofOfDeliveryScreen> createState() => _ProofOfDeliveryScreenState();
}

class _ProofOfDeliveryScreenState extends State<ProofOfDeliveryScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tab;

  @override
  void initState() {
    super.initState();
    final hasSig = widget.trip.proofSignatureUrl != null;
    final hasReceipt = widget.trip.proofReceiptUrl != null;
    final count = 1 + (hasSig ? 1 : 0) + (hasReceipt ? 1 : 0);
    _tab = TabController(length: count, vsync: this);
  }

  @override
  void dispose() {
    _tab.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final trip = widget.trip;
    final hasSig = trip.proofSignatureUrl != null;
    final hasReceipt = trip.proofReceiptUrl != null;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            const SizedBox(height: 4),
            TabBar(
              controller: _tab,
              isScrollable: true,
              tabAlignment: TabAlignment.start,
              labelColor: colors.brandTeal600,
              unselectedLabelColor: colors.textMuted,
              labelStyle: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w700,
              ),
              unselectedLabelStyle: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w500,
              ),
              indicatorColor: colors.brandTeal500,
              indicatorWeight: 2.5,
              indicatorSize: TabBarIndicatorSize.label,
              dividerColor: colors.divider,
              tabs: [
                const Tab(text: 'Photos'),
                if (hasSig) const Tab(text: 'Signature'),
                if (hasReceipt) const Tab(text: 'Receipt'),
              ],
            ),
            Expanded(
              child: TabBarView(
                controller: _tab,
                children: [
                  _PhotosTab(photos: trip.proofPhotos),
                  if (hasSig) _ImageTab(url: trip.proofSignatureUrl!),
                  if (hasReceipt) _ImageTab(url: trip.proofReceiptUrl!),
                ],
              ),
            ),
            if (trip.proofPhotos.isNotEmpty) _buildDownloadBar(context),
          ],
        ),
      ),
    );
  }

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Proof of delivery',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  Widget _buildDownloadBar(BuildContext context) {
    final colors = context.palette;
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
        decoration: BoxDecoration(
          color: colors.surface,
          border: Border(top: BorderSide(color: colors.divider)),
        ),
        child: SizedBox(
          width: double.infinity,
          height: 52,
          child: OutlinedButton.icon(
            onPressed: () {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Download started.')),
              );
            },
            icon: Icon(
              Icons.download_rounded,
              size: 18,
              color: colors.brandTeal600,
            ),
            label: Text(
              'Download all photos',
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w700,
                color: colors.brandTeal600,
              ),
            ),
            style: OutlinedButton.styleFrom(
              side: BorderSide(color: colors.brandTeal500, width: 1.4),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(14),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

// ── Tabs ────────────────────────────────────────────────────────────

class _PhotosTab extends StatelessWidget {
  const _PhotosTab({required this.photos});
  final List<String> photos;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    if (photos.isEmpty) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 40),
          child: Text(
            'No photos were captured for this delivery.',
            textAlign: TextAlign.center,
            style: TextStyle(fontSize: 14, color: colors.textMuted),
          ),
        ),
      );
    }

    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
      itemCount: photos.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, i) => _PhotoPlaceholder(
        label: photos[i],
        index: i,
        total: photos.length,
      ),
    );
  }
}

class _PhotoPlaceholder extends StatelessWidget {
  const _PhotoPlaceholder({
    required this.label,
    required this.index,
    required this.total,
  });

  final String label;
  final int index;
  final int total;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Container(
      height: 220,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: colors.navy10,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.image_outlined,
            size: 44,
            color: colors.navy40,
          ),
          const SizedBox(height: 8),
          Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: colors.textMuted,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            '${index + 1} of $total',
            style: TextStyle(
              fontSize: 11.5,
              color: colors.textMuted,
            ),
          ),
        ],
      ),
    );
  }
}

class _ImageTab extends StatelessWidget {
  const _ImageTab({required this.url});
  final String url;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.all(20),
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: colors.divider),
        ),
        alignment: Alignment.center,
        child: Icon(
          Icons.image_outlined,
          size: 60,
          color: colors.navy40,
        ),
      ),
    );
  }
}