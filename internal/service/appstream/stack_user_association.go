package appstream

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/appstream"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
)

func ResourceUserStackAssociation() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceUserStackAssociationCreate,
		ReadWithoutTimeout:   resourceUserStackAssociationRead,
		DeleteWithoutTimeout: resourceUserStackAssociationDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"authentication_type": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringInSlice(appstream.AuthenticationType_Values(), false),
			},
			"stack_name": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validation.StringLenBetween(1, 128),
			},
			"user_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceUserStackAssociationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).AppStreamConn

	input := &appstream.UserStackAssociation{
		AuthenticationType: aws.String(d.Get("authentication_type").(string)),
		StackName:          aws.String(d.Get("stack_name").(string)),
		UserName:           aws.String(d.Get("user_name").(string)),
	}

	var output *appstream.BatchAssociateUserStackOutput
	output, err := conn.BatchAssociateUserStackWithContext(ctx, &appstream.BatchAssociateUserStackInput{
		UserStackAssociations: []*appstream.UserStackAssociation{input},
	})

	if err != nil {
		return diag.FromErr(fmt.Errorf("error creating Appstream Stack User Association (%s): %w", d.Id(), err))
	}
	if len(output.Errors) > 0 {
		var errs *multierror.Error

		for _, err := range output.Errors {
			errs = multierror.Append(errs, fmt.Errorf("%s: %s", aws.StringValue(err.ErrorCode), aws.StringValue(err.ErrorMessage)))
		}
	}

	d.SetId(EncodeStackUserID(d.Get("stack_name").(string), d.Get("user_name").(string), d.Get("authentication_type").(string)))

	return resourceUserStackAssociationRead(ctx, d, meta)
}

func resourceUserStackAssociationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).AppStreamConn

	stackName, userName, authType, err := DecodeStackUserID(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("error decoding id Appstream Stack User Association (%s): %w", d.Id(), err))
	}

	resp, err := conn.DescribeUserStackAssociationsWithContext(ctx,
		&appstream.DescribeUserStackAssociationsInput{
			AuthenticationType: aws.String(authType),
			StackName:          aws.String(stackName),
			UserName:           aws.String(userName),
		})

	if !d.IsNewResource() && tfawserr.ErrCodeEquals(err, appstream.ErrCodeResourceNotFoundException) {
		log.Printf("[WARN] Appstream Stack User Association (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if len(resp.UserStackAssociations) == 0 {
		log.Printf("[WARN] Appstream User (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	association := resp.UserStackAssociations[0]

	d.Set("authentication_type", association.AuthenticationType)
	d.Set("stack_name", association.StackName)
	d.Set("user_name", association.UserName)

	return nil
}

func resourceUserStackAssociationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).AppStreamConn

	stackName, userName, authType, err := DecodeStackUserID(d.Id())
	if err != nil {
		return diag.FromErr(fmt.Errorf("error decoding id Appstream Stack User Association (%s): %w", d.Id(), err))
	}

	input := &appstream.UserStackAssociation{
		AuthenticationType: aws.String(authType),
		StackName:          aws.String(stackName),
		UserName:           aws.String(userName),
	}

	_, err = conn.BatchDisassociateUserStackWithContext(ctx, &appstream.BatchDisassociateUserStackInput{
		UserStackAssociations: []*appstream.UserStackAssociation{input},
	})

	if err != nil {
		if tfawserr.ErrCodeEquals(err, appstream.ErrCodeResourceNotFoundException) {
			return nil
		}
		return diag.FromErr(fmt.Errorf("error deleting Appstream Stack User Association (%s): %w", d.Id(), err))
	}
	return nil
}

func EncodeStackUserID(stackName, userName, authType string) string {
	return fmt.Sprintf("%s/%s/%s", stackName, userName, authType)
}

func DecodeStackUserID(id string) (string, string, string, error) {
	idParts := strings.SplitN(id, "/", 3)
	if len(idParts) != 3 {
		return "", "", "", fmt.Errorf("expected ID in format StackName/UserName/AuthenticationType, received: %s", id)
	}
	return idParts[0], idParts[1], idParts[2], nil
}
