import matplotlib.pyplot as plt

sizes = ['1x', '5x', '10x', '20x']

# Single machine times (seconds)
single_times = [0.009, 0.044, 0.071, 0.144]

# MapReduce step times (seconds)
split_times  = [0.325, 0.398, 0.448, 0.758]
map_times    = [0.138, 0.246, 0.404, 0.827]
reduce_times = [0.182, 0.195, 0.202, 0.178]
mr_times     = [s + m + r for s, m, r in zip(split_times, map_times, reduce_times)]

fig, axes = plt.subplots(1, 3, figsize=(18, 5))

# Plot 1: Single Machine vs MapReduce
x = range(len(sizes))
w = 0.35
bars1 = axes[0].bar([i - w/2 for i in x], single_times, w, label='Single Machine', color='#9C27B0')
bars2 = axes[0].bar([i + w/2 for i in x], mr_times, w, label='MapReduce (3 mappers)', color='#2196F3')
axes[0].set_xlabel('File Size')
axes[0].set_ylabel('Time (seconds)')
axes[0].set_title('Single Machine vs MapReduce')
axes[0].set_xticks(x)
axes[0].set_xticklabels(sizes)
axes[0].legend()
for bar in bars1:
    axes[0].text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.02, f'{bar.get_height():.3f}s', ha='center', fontsize=8)
for bar in bars2:
    axes[0].text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.02, f'{bar.get_height():.3f}s', ha='center', fontsize=8)

# Plot 2: MapReduce breakdown by step
bottom1 = split_times
bottom2 = [s + m for s, m in zip(split_times, map_times)]
axes[1].bar(sizes, split_times, label='Split', color='#4CAF50')
axes[1].bar(sizes, map_times, bottom=bottom1, label='Map (parallel)', color='#2196F3')
axes[1].bar(sizes, reduce_times, bottom=bottom2, label='Reduce', color='#FF9800')
axes[1].set_xlabel('File Size')
axes[1].set_ylabel('Time (seconds)')
axes[1].set_title('MapReduce Step Breakdown')
axes[1].legend()

# Plot 3: Scaling comparison - how each approach scales
axes[2].plot(sizes, single_times, 'o-', label='Single Machine', color='#9C27B0', linewidth=2)
axes[2].plot(sizes, mr_times, 's-', label='MapReduce Total', color='#2196F3', linewidth=2)
axes[2].plot(sizes, map_times, '^--', label='Map Phase Only', color='#4CAF50', linewidth=2)
axes[2].set_xlabel('File Size')
axes[2].set_ylabel('Time (seconds)')
axes[2].set_title('Scaling: Time vs File Size')
axes[2].legend()
axes[2].grid(True, alpha=0.3)

plt.tight_layout()
plt.savefig('mapreduce_results.png', dpi=150)
plt.show()
print("Saved to mapreduce_results.png")