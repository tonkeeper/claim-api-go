// Code generated by ogen, DO NOT EDIT.

package oas

import (
	"net/http"
	"net/url"

	"github.com/go-faster/errors"

	"github.com/ogen-go/ogen/conv"
	"github.com/ogen-go/ogen/middleware"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/ogen-go/ogen/uri"
	"github.com/ogen-go/ogen/validate"
)

// GetWalletInfoParams is parameters of getWalletInfo operation.
type GetWalletInfoParams struct {
	Address string
}

func unpackGetWalletInfoParams(packed middleware.Parameters) (params GetWalletInfoParams) {
	{
		key := middleware.ParameterKey{
			Name: "address",
			In:   "path",
		}
		params.Address = packed[key].(string)
	}
	return params
}

func decodeGetWalletInfoParams(args [1]string, argsEscaped bool, r *http.Request) (params GetWalletInfoParams, _ error) {
	// Decode path: address.
	if err := func() error {
		param := args[0]
		if argsEscaped {
			unescaped, err := url.PathUnescape(args[0])
			if err != nil {
				return errors.Wrap(err, "unescape path")
			}
			param = unescaped
		}
		if len(param) > 0 {
			d := uri.NewPathDecoder(uri.PathDecoderConfig{
				Param:   "address",
				Value:   param,
				Style:   uri.PathStyleSimple,
				Explode: false,
			})

			if err := func() error {
				val, err := d.DecodeValue()
				if err != nil {
					return err
				}

				c, err := conv.ToString(val)
				if err != nil {
					return err
				}

				params.Address = c
				return nil
			}(); err != nil {
				return err
			}
		} else {
			return validate.ErrFieldRequired
		}
		return nil
	}(); err != nil {
		return params, &ogenerrors.DecodeParamError{
			Name: "address",
			In:   "path",
			Err:  err,
		}
	}
	return params, nil
}