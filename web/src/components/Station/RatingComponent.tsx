import React, { useState, useEffect } from 'react';
import { Star } from 'lucide-react';
import { api, fetchCSRFToken, base64ToHex } from '../../lib/api';
import { useAuthStore } from '../../stores/authStore';

interface RatingComponentProps {
  stationId: number;
  stationUUID?: string; // Required for federated stations
  isFederated?: boolean; // True if this is a federated station
  isAuthenticated: boolean;
  ownerPubkey: string;
  initialAverageRating?: number; // Initial rating value from station data
  initialVoteCount?: number; // Initial vote count from station data
  onRatingSubmitted?: () => void;
}

interface RatingStats {
  average_rating: number;
  vote_count: number;
}

interface RatingResponse {
  success: boolean;
  rating: number;
  average_rating: number;
  vote_count: number;
}

const RatingComponent: React.FC<RatingComponentProps> = ({
  stationId,
  stationUUID,
  isFederated = false,
  isAuthenticated,
  ownerPubkey,
  initialAverageRating,
  initialVoteCount,
  onRatingSubmitted,
}) => {
  const [stats, setStats] = useState<RatingStats | null>(null);
  const [hoverRating, setHoverRating] = useState(0);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { publicKey } = useAuthStore();

  // Check if current user is the owner
  const isOwner = !!(isAuthenticated && publicKey && base64ToHex(publicKey) === ownerPubkey);

  // Fetch rating stats (public) - only for local stations
  const fetchRatingStats = async () => {
    // For federated stations, use initial values from station data
    if (isFederated) {
      setStats({
        average_rating: initialAverageRating || 0,
        vote_count: initialVoteCount || 0,
      });
      return;
    }

    // For local stations, fetch from API
    try {
      const data = await api.getRatingStats<RatingStats>(stationId);
      setStats(data);
    } catch (err) {
      console.error('Failed to fetch rating stats:', err);
      // Don't show error to user for stats fetch failure
      setStats({ average_rating: 0, vote_count: 0 });
    }
  };

  // Load data on mount
  useEffect(() => {
    fetchRatingStats();
  }, [stationId, isFederated, initialAverageRating, initialVoteCount]);

  // Handle rating submission
  const handleRating = async (rating: number) => {
    if (!isAuthenticated) {
      setError('Sign in to rate');
      return;
    }

    if (isOwner) {
      setError("You can't rate your own station");
      return;
    }

    // For federated stations, ensure UUID is provided
    if (isFederated && !stationUUID) {
      setError('Station UUID is required for federated stations');
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const csrfToken = await fetchCSRFToken();

      // Use different API methods for local vs federated stations
      if (isFederated && stationUUID) {
        // For federated stations, forward rating to source node
        await api.rateFederatedStation<RatingResponse>(
          stationUUID,
          rating,
          csrfToken
        );

        // For federated stations, we won't get updated stats immediately
        // Just show success and keep current stats
        setStats({
          average_rating: stats?.average_rating || 0,
          vote_count: stats?.vote_count || 0,
        });
      } else {
        // For local stations, use regular endpoint
        const data = await api.rateStation<RatingResponse>(
          stationId,
          rating,
          csrfToken
        );

        // Update local state with new stats
        setStats({
          average_rating: data.average_rating,
          vote_count: data.vote_count,
        });
      }

      // Notify parent component
      if (onRatingSubmitted) {
        onRatingSubmitted();
      }
    } catch (err) {
      console.error('Failed to submit rating:', err);
      setError('Failed to submit rating. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  // Display rating: hover preview OR average rating
  const displayRating = hoverRating || stats?.average_rating || 0;

  // Helper function to round rating to nearest quarter
  const roundToQuarter = (value: number): number => {
    return Math.round(value * 4) / 4;
  };

  // Helper function to determine star fill
  const getStarFill = (starIndex: number): 'full' | 'partial' | 'empty' => {
    const rating = roundToQuarter(displayRating);
    if (starIndex <= Math.floor(rating)) {
      return 'full';
    } else if (starIndex === Math.ceil(rating) && rating % 1 !== 0) {
      return 'partial';
    }
    return 'empty';
  };

  // Get fill percentage for partial stars (rounded to quarters: 25%, 50%, 75%)
  const getPartialFillPercentage = (starIndex: number): number => {
    const rating = roundToQuarter(displayRating);
    if (starIndex === Math.ceil(rating)) {
      const fraction = rating % 1;
      // Round to nearest quarter: 0.25 -> 25%, 0.5 -> 50%, 0.75 -> 75%
      return fraction * 100;
    }
    return 0;
  };

  return (
    <div className="space-y-2">
      {/* Star rating input */}
      <div className="flex items-center gap-1">
        {[1, 2, 3, 4, 5].map((star) => {
          const fillType = getStarFill(star);
          const fillPercentage = getPartialFillPercentage(star);
          const isDisabled: boolean = !isAuthenticated || isLoading || isOwner;

          return (
            <button
              key={star}
              onClick={() => handleRating(star)}
              onMouseEnter={() => !isDisabled && setHoverRating(star)}
              onMouseLeave={() => setHoverRating(0)}
              disabled={isDisabled}
              className={`transition-transform ${
                isDisabled
                  ? 'cursor-not-allowed opacity-50'
                  : 'cursor-pointer hover:scale-110'
              }`}
              aria-label={`Rate ${star} stars`}
            >
              <div className="relative w-5 h-5">
                {fillType === 'full' && (
                  <Star
                    className="w-5 h-5 text-yellow-400"
                    fill="currentColor"
                    strokeWidth={2}
                  />
                )}
                {fillType === 'empty' && (
                  <Star
                    className="w-5 h-5 text-gray-400 dark:text-gray-600"
                    fill="none"
                    strokeWidth={2}
                  />
                )}
                {fillType === 'partial' && (
                  <>
                    {/* Background empty star */}
                    <Star
                      className="w-5 h-5 text-gray-400 dark:text-gray-600 absolute top-0 left-0"
                      fill="none"
                      strokeWidth={2}
                    />
                    {/* Clipped filled star */}
                    <div
                      className="absolute top-0 left-0 overflow-hidden"
                      style={{ width: `${fillPercentage}%` }}
                    >
                      <Star
                        className="w-5 h-5 text-yellow-400"
                        fill="currentColor"
                        strokeWidth={2}
                      />
                    </div>
                  </>
                )}
              </div>
            </button>
          );
        })}
      </div>

      {/* Rating stats display */}
      {stats && (
        <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
          <span className="font-medium">
            {stats.average_rating > 0
              ? `${stats.average_rating.toFixed(2)} / 5.00`
              : 'No ratings'}
          </span>
          {stats.vote_count > 0 && (
            <span className="text-gray-500 dark:text-gray-500">
              ({stats.vote_count} {stats.vote_count === 1 ? 'vote' : 'votes'})
            </span>
          )}
        </div>
      )}

      {/* Error or info message */}
      {error && (
        <p className="text-sm text-red-500 dark:text-red-400">{error}</p>
      )}
      {!error && isOwner && (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          You can't rate your own station
        </p>
      )}
      {!error && !isAuthenticated && !isOwner && (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Sign in to rate
        </p>
      )}

      {/* Loading indicator */}
      {isLoading && (
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Submitting...
        </p>
      )}
    </div>
  );
};

export default RatingComponent;
