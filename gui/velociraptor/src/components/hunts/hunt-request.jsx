import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import VeloAce from '../core/ace.jsx';

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
                <Card.Header>{T("Request sent to client")}</Card.Header>
                <Card.Body>
                  <VeloAce text={serialized}
                           options={options} />
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
};
