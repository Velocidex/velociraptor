import React from 'react';
import PropTypes from 'prop-types';

import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import UserConfig from '../core/user.js';

export default class UserLabel extends React.Component {
    static contextType = UserConfig;
    static propTypes = {

    };

    render() {
        return (
            <>
              <ButtonGroup>
                <Button href={"/logoff?username="+ this.context.traits.username }>
                  <FontAwesomeIcon icon="sign-out-alt" />
                </Button>
                <Button variant="default">
                  { this.context.traits.username }
                  { this.context.traits.picture &&
                    <img className="toolbar-buttons"
                         src={ this.context.traits.picture} className="img-thumbnail"
                    />
                  }
                </Button>
              </ButtonGroup>
            </>
        );
    }
};
