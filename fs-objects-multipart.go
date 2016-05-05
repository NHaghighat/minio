/*
 * Minio Cloud Storage, (C) 2016 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"io"
	"path"
)

// ListMultipartUploads - list multipart uploads.
func (fs fsObjects) ListMultipartUploads(bucket, prefix, keyMarker, uploadIDMarker, delimiter string, maxUploads int) (ListMultipartsInfo, error) {
	return listMultipartUploadsCommon(fs, bucket, prefix, keyMarker, uploadIDMarker, delimiter, maxUploads)
}

// NewMultipartUpload - initialize a new multipart upload, returns a unique id.
func (fs fsObjects) NewMultipartUpload(bucket, object string) (string, error) {
	return newMultipartUploadCommon(fs.storage, bucket, object)
}

// PutObjectPart - writes the multipart upload chunks.
func (fs fsObjects) PutObjectPart(bucket, object, uploadID string, partID int, size int64, data io.Reader, md5Hex string) (string, error) {
	return putObjectPartCommon(fs.storage, bucket, object, uploadID, partID, size, data, md5Hex)
}

func (fs fsObjects) ListObjectParts(bucket, object, uploadID string, partNumberMarker, maxParts int) (ListPartsInfo, error) {
	return listObjectPartsCommon(fs.storage, bucket, object, uploadID, partNumberMarker, maxParts)
}

func (fs fsObjects) CompleteMultipartUpload(bucket string, object string, uploadID string, parts []completePart) (string, error) {
	// Verify if bucket is valid.
	if !IsValidBucketName(bucket) {
		return "", BucketNameInvalid{Bucket: bucket}
	}
	if !IsValidObjectName(object) {
		return "", ObjectNameInvalid{
			Bucket: bucket,
			Object: object,
		}
	}
	if status, err := isUploadIDExists(fs.storage, bucket, object, uploadID); err != nil {
		return "", err
	} else if !status {
		return "", InvalidUploadID{UploadID: uploadID}
	}

	tempObj := path.Join(tmpMetaPrefix, bucket, object, uploadID)
	fileWriter, err := fs.storage.CreateFile(minioMetaBucket, tempObj)
	if err != nil {
		return "", toObjectErr(err, bucket, object)
	}

	var md5Sums []string
	for _, part := range parts {
		// Construct part suffix.
		partSuffix := fmt.Sprintf("%s.%.5d.%s", uploadID, part.PartNumber, part.ETag)
		multipartPartFile := path.Join(mpartMetaPrefix, bucket, object, partSuffix)
		var fileReader io.ReadCloser
		fileReader, err = fs.storage.ReadFile(minioMetaBucket, multipartPartFile, 0)
		if err != nil {
			if err == errFileNotFound {
				return "", InvalidPart{}
			}
			return "", err
		}
		_, err = io.Copy(fileWriter, fileReader)
		if err != nil {
			return "", err
		}
		err = fileReader.Close()
		if err != nil {
			return "", err
		}
		md5Sums = append(md5Sums, part.ETag)
	}

	err = fileWriter.Close()
	if err != nil {
		return "", err
	}

	// Save the s3 md5.
	s3MD5, err := makeS3MD5(md5Sums...)
	if err != nil {
		return "", err
	}

	// Cleanup all the parts.
	if err = cleanupUploadedParts(fs.storage, mpartMetaPrefix, bucket, object, uploadID); err != nil {
		return "", err
	}

	err = fs.storage.RenameFile(minioMetaBucket, tempObj, bucket, object)
	if err != nil {
		return "", toObjectErr(err, bucket, object)
	}

	// Return md5sum.
	return s3MD5, nil
}

// AbortMultipartUpload - aborts a multipart upload.
func (fs fsObjects) AbortMultipartUpload(bucket, object, uploadID string) error {
	return abortMultipartUploadCommon(fs.storage, bucket, object, uploadID)
}