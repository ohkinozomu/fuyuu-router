package common

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type CommonConfig struct {
	MQTTBroker string
	Username   string
	Password   string
	Logger     *zap.Logger
	CAFile     string
	Cert       string
	Key        string
	Protocol   string
}

type Profiling struct {
	Registry      string `mapstructure:"registry" validate:"omitempty,oneof=cloudprofiler pyroscope"`
	ServerAddress string `mapstructure:"server_address"`
}

type Networking struct {
	Format          string `mapstructure:"format" validate:"omitempty,oneof=json protobuf"`
	Compress        string `mapstructure:"compress" validate:"omitempty,oneof=none zstd"`
	LargeDataPolicy string `mapstructure:"large_data_policy" validate:"omitempty,oneof=none storage_relay"`
}

type StorageRelay struct {
	ThresholdBytes int    `mapstructure:"threshold_bytes"`
	ObjstoreFile   string `mapstructure:"objstore_file"`
}

type CommonConfigV2 struct {
	Profiling    Profiling    `mapstructure:"profiling"`
	Networking   Networking   `mapstructure:"networking"`
	StorageRelay StorageRelay `mapstructure:"storage_relay"`
}

func CreateConfig(configPath string) (CommonConfigV2, error) {
	var config CommonConfigV2

	viper.SetDefault("networking.format", "json")
	viper.SetDefault("networking.compress", "none")
	viper.SetDefault("networking.large_data_policy", "none")
	// based on AWS IoT Core's limit: https://docs.aws.amazon.com/general/latest/gr/iot-core.html#message-broker-limits
	viper.SetDefault("storage_relay.threshold_bytes", 128000)
	viper.SetConfigFile(configPath)

	if err := viper.ReadInConfig(); err != nil {
		return config, err
	}

	if err := viper.Unmarshal(&config); err != nil {
		return config, err
	}

	validate := validator.New()
	if err := validate.Struct(config); err != nil {
		return config, fmt.Errorf("error validating config: %w", err)
	}

	return config, nil
}
