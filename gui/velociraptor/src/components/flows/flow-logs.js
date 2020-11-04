import React from 'react';
import PropTypes from 'prop-types';

import VeloPagedTable from '../core/paged-table.js';
import VeloTimestamp from "../utils/time.js";
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

export default class FlowLogs extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        let renderers = {
            Timestamp: (cell, row, rowIndex) => {
                return (
                    <VeloTimestamp usec={cell / 1000}/>
                );
            },
        };

        return (
            <CardDeck>
              <Card>
                <Card.Header>Query logs</Card.Header>
                <Card.Body>
                  <VeloPagedTable
                    className="col-12"
                    renderers={renderers}
                    params={{
                        client_id: this.props.flow.client_id,
                        flow_id: this.props.flow.session_id,
                        type: "log",
                    }}
                  />
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
}
