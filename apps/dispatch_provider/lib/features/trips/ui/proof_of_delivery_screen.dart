import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/dispatch_empty_state.dart';
import '../../../shared/widgets/journey_icon.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/trip_model.dart';

class ProofOfDeliveryScreen extends StatefulWidget {
  const ProofOfDeliveryScreen({super.key, required this.trip});
  final TripModel trip;
  @override
  State<ProofOfDeliveryScreen> createState() => _ProofOfDeliveryScreenState();
}

class _ProofOfDeliveryScreenState extends State<ProofOfDeliveryScreen> {
  int _selected = 0, _revision = 0;
  @override
  Widget build(BuildContext context) {
    final trip = widget.trip;
    final assets = <({String title, String url})>[
      for (var i = 0; i < trip.proofPhotos.length; i++)
        (title: 'Photo ${i + 1}', url: trip.proofPhotos[i]),
      if (trip.proofPhotos.isEmpty && trip.proofUrl != null)
        (title: 'Photo', url: trip.proofUrl!),
      if (trip.proofSignatureUrl != null)
        (title: 'Signature', url: trip.proofSignatureUrl!),
      if (trip.proofReceiptUrl != null)
        (title: 'Receipt', url: trip.proofReceiptUrl!),
    ];
    final record = trip.proofRecord;
    final submitted = DateTime.tryParse(
      record?['submitted_at']?.toString() ?? '',
    );
    return Scaffold(
      backgroundColor: context.palette.background,
      appBar: AppBar(
        leading: const DispatchBackButton(),
        title: const Text('Proof of delivery'),
      ),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
        children: [
          DeliveryPanel(
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const JourneyIcon(JourneySymbol.shield, size: 34),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        record?['verified'] == true
                            ? 'Verified handover'
                            : 'Saved delivery evidence',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 6),
                      Text(
                        trip.receiverName.isNotEmpty
                            ? trip.receiverName
                            : 'Delivery record',
                        style: Theme.of(context).textTheme.bodyMedium,
                      ),
                      if (submitted != null) ...[
                        const SizedBox(height: 4),
                        Text(
                          deliveryDate(submitted, time: true),
                          style: Theme.of(context).textTheme.bodySmall,
                        ),
                      ],
                    ],
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 20),
          if (assets.isEmpty)
            const DispatchEmptyState(
              story: EmptyStory.activity,
              title: 'Preview unavailable',
              message:
                  'The record has no viewable evidence files available here.',
            )
          else ...[
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  for (var i = 0; i < assets.length; i++)
                    Padding(
                      padding: const EdgeInsets.only(right: 8),
                      child: DeliveryFilter(
                        label: assets[i].title,
                        selected: _selected == i,
                        onTap: () => setState(() => _selected = i),
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            Text('Pinch to zoom', style: Theme.of(context).textTheme.bodySmall),
            const SizedBox(height: 10),
            ClipRRect(
              borderRadius: BorderRadius.circular(24),
              child: Container(
                color: context.palette.surfaceElevated,
                child: AspectRatio(
                  aspectRatio: 4 / 5,
                  child: InteractiveViewer(
                    minScale: 1,
                    maxScale: 4,
                    child: _image(assets[_selected].url),
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _image(String url) {
    final uri = Uri.tryParse(url);
    if (uri == null || !['http', 'https'].contains(uri.scheme))
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(24),
          child: Text('This evidence file does not have a viewable link.'),
        ),
      );
    return Image.network(
      url,
      key: ValueKey('$url:$_revision'),
      fit: BoxFit.contain,
      loadingBuilder: (context, child, progress) => progress == null
          ? child
          : const Center(
              child: SizedBox.square(
                dimension: 24,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            ),
      errorBuilder: (context, _, _) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: ActionNotice(
            message: 'Could not load this evidence file.',
            isError: true,
            onRetry: () async {
              await NetworkImage(url).evict();
              if (mounted) setState(() => _revision++);
            },
          ),
        ),
      ),
    );
  }
}
