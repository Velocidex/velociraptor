import React from 'react';
import PropTypes from 'prop-types';

import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import VeloAce from '../core/ace.js';

export default class HuntRequest extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    render() {
        let serialized = JSON.stringify(this.props.hunt.start_request, null, 2);
        let options = {
            readOnly: true,
        };

        return (
            <CardDeck>
              <Card>
                <Card.Header>Request sent to client</Card.Header>
                <Card.Body>
                  <VeloAce text={serialized}
                           options={options} />
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
};
