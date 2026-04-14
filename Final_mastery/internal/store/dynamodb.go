package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBStore provides DynamoDB-backed storage
type DynamoDBStore struct {
	client      *dynamodb.Client
	albumsTable string
	photosTable string
}

// NewDynamoDBStore creates a new DynamoDB store
func NewDynamoDBStore(client *dynamodb.Client, albumsTable, photosTable string) *DynamoDBStore {
	return &DynamoDBStore{
		client:      client,
		albumsTable: albumsTable,
		photosTable: photosTable,
	}
}

// Helper function to get keys from attribute map
func getKeys(m map[string]types.AttributeValue) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// PutAlbum stores or updates an album
func (s *DynamoDBStore) PutAlbum(album *Album) error {
	item, err := attributevalue.MarshalMap(album)
	if err != nil {
		return fmt.Errorf("failed to marshal album: %w", err)
	}

	// Debug: check if album_id is in the item
	if _, ok := item["album_id"]; !ok {
		return fmt.Errorf("album_id key missing after marshal, keys present: %v", getKeys(item))
	}

	_, err = s.client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(s.albumsTable),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put album: %w", err)
	}

	return nil
}

// GetAlbum retrieves an album by ID (strongly consistent)
func (s *DynamoDBStore) GetAlbum(albumID string) (*Album, error) {
	result, err := s.client.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(s.albumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get album: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var album Album
	if err := attributevalue.UnmarshalMap(result.Item, &album); err != nil {
		return nil, fmt.Errorf("failed to unmarshal album: %w", err)
	}

	return &album, nil
}

// ListAlbums returns all albums with full pagination
func (s *DynamoDBStore) ListAlbums() ([]*Album, error) {
	var albums []*Album
	var lastKey map[string]types.AttributeValue

	for {
		input := &dynamodb.ScanInput{
			TableName: aws.String(s.albumsTable),
		}
		if lastKey != nil {
			input.ExclusiveStartKey = lastKey
		}

		result, err := s.client.Scan(context.TODO(), input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan albums: %w", err)
		}

		for _, item := range result.Items {
			var album Album
			if err := attributevalue.UnmarshalMap(item, &album); err != nil {
				return nil, fmt.Errorf("failed to unmarshal album: %w", err)
			}
			albums = append(albums, &album)
		}

		if result.LastEvaluatedKey == nil {
			break
		}
		lastKey = result.LastEvaluatedKey
	}

	return albums, nil
}

// IncrementSeq atomically increments and returns the seq for an album
func (s *DynamoDBStore) IncrementSeq(albumID string) (int, error) {
	seqKey := fmt.Sprintf("SEQ#%s", albumID)

	result, err := s.client.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName: aws.String(s.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: seqKey},
		},
		UpdateExpression: aws.String("ADD seq_counter :inc"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":inc": &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to increment seq: %w", err)
	}

	var seqCounter int
	if val, ok := result.Attributes["seq_counter"]; ok {
		if err := attributevalue.Unmarshal(val, &seqCounter); err != nil {
			return 0, fmt.Errorf("failed to unmarshal seq counter: %w", err)
		}
	}

	return seqCounter, nil
}

// PutPhoto stores or updates a photo
func (s *DynamoDBStore) PutPhoto(photo *Photo) error {
	item, err := attributevalue.MarshalMap(photo)
	if err != nil {
		return fmt.Errorf("failed to marshal photo: %w", err)
	}

	_, err = s.client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(s.photosTable),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put photo: %w", err)
	}

	return nil
}

// GetPhoto retrieves a photo by ID (strongly consistent to see deletes immediately)
func (s *DynamoDBStore) GetPhoto(photoID string) (*Photo, error) {
	result, err := s.client.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName: aws.String(s.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get photo: %w", err)
	}

	if result.Item == nil {
		return nil, nil
	}

	var photo Photo
	if err := attributevalue.UnmarshalMap(result.Item, &photo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal photo: %w", err)
	}

	return &photo, nil
}

// DeletePhoto removes a photo
func (s *DynamoDBStore) DeletePhoto(photoID string) error {
	_, err := s.client.DeleteItem(context.TODO(), &dynamodb.DeleteItemInput{
		TableName: aws.String(s.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete photo: %w", err)
	}

	return nil
}

// UpdatePhotoStatus updates the status of a photo only if the record still exists.
// Uses a conditional expression to prevent creating zombie records after delete.
func (s *DynamoDBStore) UpdatePhotoStatus(photoID, status string) error {
	_, err := s.client.UpdateItem(context.TODO(), &dynamodb.UpdateItemInput{
		TableName: aws.String(s.photosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		UpdateExpression:    aws.String("SET #status = :status"),
		ConditionExpression: aws.String("attribute_exists(photo_id)"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: status},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update photo status: %w", err)
	}

	return nil
}
