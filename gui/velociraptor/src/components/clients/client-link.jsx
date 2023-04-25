import './client-link.css';

import React from 'react';
import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import { withRouter }  from "react-router-dom";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

class ClientLink extends React.Component {
    static propTypes = {
        client_id: PropTypes.string,

        // React router props.
        history: PropTypes.object,
    };

    link = () => {
        this.props.history.push('/host/' + this.props.client_id);
    }

    render() {
        return (
            <>
              <Button size="sm" className="client-link"
                      id={this.props.client_id}
                      onClick={this.link}
                      variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                <span className="button-label">{ this.props.client_id }</span>
              </Button>
            </>
        );
    }
};


export default withRouter(ClientLink);
